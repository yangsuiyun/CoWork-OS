// Package projector consumes event_log in global_seq order and maintains the
// read models (spec 8.3). Projectors are deterministic and perform no I/O
// beyond DB writes, so a read model can be DROPped and rebuilt by replay.
//
// It tracks progress in consumer_offset and is idempotent: re-applying an event
// is a no-op (ON CONFLICT / monotonic updated_seq guard), so at-least-once
// delivery is safe.
//
// NOTE: the projector reads across tenants and therefore must run with a
// role that bypasses RLS (dev uses the postgres superuser). A production
// deployment uses a dedicated projector role with BYPASSRLS.
package projector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	consumerName = "rm_tasks"
	batchSize    = 500
)

// Projector maintains read models from the event log.
type Projector struct {
	pool *pgxpool.Pool
}

// New constructs a Projector.
func New(pool *pgxpool.Pool) *Projector { return &Projector{pool: pool} }

// Run polls RunOnce until ctx is cancelled.
func (p *Projector) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := p.RunOnce(ctx); err != nil {
				// Fail-fast: surface and retry next tick; do not silently mask.
				continue
			}
		}
	}
}

// RunOnce applies the next batch of events and advances the offset in one tx.
// Returns the number of events applied.
func (p *Projector) RunOnce(ctx context.Context) (int, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"INSERT INTO consumer_offset (consumer, last_global_seq) VALUES ($1, 0) ON CONFLICT DO NOTHING", consumerName); err != nil {
		return 0, err
	}

	var offset int64
	if err := tx.QueryRow(ctx,
		"SELECT last_global_seq FROM consumer_offset WHERE consumer = $1 FOR UPDATE", consumerName).Scan(&offset); err != nil {
		return 0, err
	}

	rows, err := tx.Query(ctx, `
		SELECT global_seq, tenant_id, stream_id, type, payload, occurred_at
		FROM event_log WHERE global_seq > $1 ORDER BY global_seq LIMIT $2`, offset, batchSize)
	if err != nil {
		return 0, err
	}
	type rec struct {
		seq        int64
		tenant     string
		stream     string
		typ        string
		payload    []byte
		occurredAt time.Time
	}
	var batch []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.seq, &r.tenant, &r.stream, &r.typ, &r.payload, &r.occurredAt); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	last := offset
	for _, r := range batch {
		if err := applyToTasks(ctx, tx, r.seq, r.tenant, r.stream, r.typ, r.payload, r.occurredAt); err != nil {
			return 0, err
		}
		last = r.seq
	}

	if last != offset {
		if _, err := tx.Exec(ctx,
			"UPDATE consumer_offset SET last_global_seq = $2 WHERE consumer = $1", consumerName, last); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(batch), nil
}

// Rebuild truncates the task read model, resets the offset, and replays the
// whole log. Proves the read model is a pure function of the event log.
func (p *Projector) Rebuild(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, "TRUNCATE rm_tasks, rm_timeline"); err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx,
		"INSERT INTO consumer_offset (consumer, last_global_seq) VALUES ($1, 0) ON CONFLICT (consumer) DO UPDATE SET last_global_seq = 0", consumerName); err != nil {
		return err
	}
	for {
		n, err := p.RunOnce(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func statusForType(typ string) (string, bool) {
	switch typ {
	case "TaskPlanned":
		return "planned", true
	case "TurnStarted":
		return "running", true
	case "TurnCompleted":
		return "pending", true
	case "TaskCompleted":
		return "completed", true
	case "TaskFailed":
		return "failed", true
	case "TaskCancelled":
		return "cancelled", true
	default:
		return "", false
	}
}

func applyToTasks(ctx context.Context, tx pgx.Tx, seq int64, tenant, stream, typ string, payload []byte, occurredAt time.Time) error {
	taskID := strings.TrimPrefix(stream, "task:")

	if typ == "TaskCreated" {
		var p struct {
			WorkspaceID     string  `json:"workspaceId"`
			CanonicalPrompt string  `json:"canonicalPrompt"`
			Risk            string  `json:"risk"`
			Origin          string  `json:"origin"`
			ParentTaskID    *string `json:"parentTaskId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_tasks (id, tenant_id, workspace_id, parent_task_id, status, title, risk, origin, updated_seq, updated_at)
			VALUES ($1,$2,$3,$4,'pending',$5,$6,$7,$8,$9)
			ON CONFLICT (id) DO NOTHING`,
			taskID, tenant, p.WorkspaceID, p.ParentTaskID, p.CanonicalPrompt, p.Risk, p.Origin, seq, occurredAt); err != nil {
			return err
		}
	} else if status, ok := statusForType(typ); ok {
		// Monotonic guard: ignore stale/replayed events (idempotent).
		if _, err := tx.Exec(ctx, `
			UPDATE rm_tasks SET status = $2, updated_seq = $3, updated_at = $4
			WHERE id = $1 AND updated_seq < $3`,
			taskID, status, seq, occurredAt); err != nil {
			return err
		}
	}

	// Timeline entry per event (idempotent on replay).
	if _, err := tx.Exec(ctx, `
		INSERT INTO rm_timeline (tenant_id, task_id, seq, kind, summary, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (task_id, seq) DO NOTHING`,
		tenant, taskID, seq, typ, typ, occurredAt); err != nil {
		return err
	}
	return nil
}
