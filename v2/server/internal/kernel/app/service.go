// Package app is the kernel application service: it wires the Task aggregate
// (pure decide) to the event store (append + outbox) inside one tenant-scoped
// transaction, and serves read-model queries. This is the only entry the HTTP
// adapter calls (spec 6.1 narrow contract).
package app

import (
	"context"
	"errors"
	"strconv"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/task"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotifyChannel is the Postgres LISTEN/NOTIFY channel for committed events.
// The payload is the highest global_seq of the batch; listeners re-query their
// own tenant's events since their cursor (spec realtime fan-out).
const NotifyChannel = "cowork_events"

// Service handles commands and queries against the kernel.
type Service struct {
	store *events.Store
	pool  *pgxpool.Pool
}

// New constructs a Service.
func New(pool *pgxpool.Pool) *Service {
	return &Service{store: events.NewStore(pool), pool: pool}
}

// Handle processes a command end-to-end: load stream -> rebuild aggregate ->
// decide (invariants) -> append events + outbox, all in one tenant tx.
func (s *Service) Handle(ctx context.Context, tenant, actor, cmdType string, payload []byte) ([]events.Committed, error) {
	cmd, stream, err := decodeCommand(cmdType, payload)
	if err != nil {
		return nil, err
	}

	var committed []events.Committed
	err = s.store.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		history, err := s.store.LoadStream(ctx, tx, stream)
		if err != nil {
			return err
		}
		domainHist := make([]task.Event, 0, len(history))
		for _, c := range history {
			de, err := committedToEvent(c)
			if err != nil {
				return err
			}
			domainHist = append(domainHist, de)
		}

		agg := task.Load(domainHist)
		newEvents, err := agg.Decide(cmd) // domain error propagates (fail-fast)
		if err != nil {
			return err
		}

		toAppend := make([]events.ToAppend, 0, len(newEvents))
		for _, e := range newEvents {
			ta, err := eventToAppend(e, actor, nil)
			if err != nil {
				return err
			}
			toAppend = append(toAppend, ta)
		}

		var expectedSeq int64
		if n := len(history); n > 0 {
			expectedSeq = history[n-1].StreamSeq
		}
		committed, err = s.store.Append(ctx, tx, tenant, stream, expectedSeq, toAppend)
		if err != nil {
			return err
		}
		// Transactional NOTIFY: delivered on commit, wakes WS fan-out.
		if n := len(committed); n > 0 {
			_, err = tx.Exec(ctx, "SELECT pg_notify($1, $2)", NotifyChannel,
				strconv.FormatInt(committed[n-1].GlobalSeq, 10))
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return committed, nil
}

// EventsSince returns the tenant's events with global_seq greater than fromSeq,
// ordered ascending. RLS scopes the read to the tenant (no cross-tenant leak).
func (s *Service) EventsSince(ctx context.Context, tenant string, fromSeq int64, limit int) ([]events.Committed, error) {
	var out []events.Committed
	err := s.store.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT global_seq, tenant_id, stream_id, stream_seq, type, schema_ver, payload, actor, correlation_id, causation_id, occurred_at
			FROM event_log WHERE global_seq > $1 ORDER BY global_seq LIMIT $2`, fromSeq, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c events.Committed
			if err := rows.Scan(&c.GlobalSeq, &c.TenantID, &c.StreamID, &c.StreamSeq, &c.Type, &c.SchemaVer,
				&c.Payload, &c.Actor, &c.CorrelationID, &c.CausationID, &c.OccurredAt); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// TaskView is a read-model row returned by queries.
type TaskView struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Status      string `json:"status"`
	Title       string `json:"title"`
	Risk        string `json:"risk"`
	Origin      string `json:"origin"`
	UpdatedSeq  int64  `json:"updatedSeq"`
}

// QueryTasks returns the tenant's tasks from the read model (populated by the
// projector). Returns empty before the projector has run.
func (s *Service) QueryTasks(ctx context.Context, tenant string, limit int) ([]TaskView, error) {
	var out []TaskView
	err := s.store.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, workspace_id, status, COALESCE(title,''), risk, origin, updated_seq
			FROM rm_tasks ORDER BY updated_seq DESC LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v TaskView
			if err := rows.Scan(&v.ID, &v.WorkspaceID, &v.Status, &v.Title, &v.Risk, &v.Origin, &v.UpdatedSeq); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// ErrorCode classifies a Handle error into a stable contract code + HTTP-ish
// category for the adapter (single mapping site, spec P6).
func ErrorCode(err error) (code string, category string) {
	switch {
	case errors.Is(err, ErrUnknownCommand):
		return "unknown_command", "bad_request"
	case errors.Is(err, events.ErrConcurrencyConflict):
		return "concurrency_conflict", "conflict"
	case errors.Is(err, task.ErrAlreadyExists),
		errors.Is(err, task.ErrNotFound),
		errors.Is(err, task.ErrTerminal),
		errors.Is(err, task.ErrNoParent):
		return "invariant_violated", "unprocessable"
	default:
		return "internal", "internal"
	}
}
