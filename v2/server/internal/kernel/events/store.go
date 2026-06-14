// Package events implements the append-only event store: the single source of
// truth for all state changes (spec P2, 8.1). It assigns a monotonically
// VISIBLE global_seq at commit (advisory-lock serialized, not BIGSERIAL),
// writes the transactional outbox, and enforces optimistic concurrency via
// UNIQUE(stream_id, stream_seq).
package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrConcurrencyConflict is returned when an append collides on
// (stream_id, stream_seq): the stream advanced since the caller loaded it.
var ErrConcurrencyConflict = errors.New("events: stream sequence conflict")

// shardLockKey serializes global_seq assignment within a shard. A single
// logical writer holds this advisory xact lock so the committed global_seq is
// monotonically visible to cursor consumers (spec 8.1, fix for BIGSERIAL gap).
const shardLockKey int64 = 1

// ToAppend is an event the caller wants to append; the store assigns
// stream_seq, global_seq, and occurred_at.
type ToAppend struct {
	Type          string
	SchemaVer     int
	Payload       []byte // JSON
	Actor         string
	CorrelationID *string
	CausationID   *string
}

// Committed is an event as stored in event_log.
type Committed struct {
	GlobalSeq     int64
	TenantID      string
	StreamID      string
	StreamSeq     int64
	Type          string
	SchemaVer     int
	Payload       []byte
	Actor         string
	CorrelationID *string
	CausationID   *string
	OccurredAt    time.Time
}

// Store is the event store over a pgx pool.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// WithTenantTx runs fn inside a transaction with app.tenant_id set for the
// duration of the transaction, scoping RLS to the tenant (spec 11.0).
func (s *Store) WithTenantTx(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// set_config(..., is_local=true) == SET LOCAL; reverts at tx end.
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Append writes events to a stream within an existing transaction. expectedSeq
// is the stream_seq the caller observed when loading the aggregate; the first
// new event gets expectedSeq+1. A UNIQUE violation surfaces as
// ErrConcurrencyConflict so the caller can reload and retry.
func (s *Store) Append(ctx context.Context, tx pgx.Tx, tenantID, streamID string, expectedSeq int64, evs []ToAppend) ([]Committed, error) {
	if len(evs) == 0 {
		return nil, nil
	}
	// Serialize global_seq assignment so committed order == seq order.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", shardLockKey); err != nil {
		return nil, err
	}

	out := make([]Committed, 0, len(evs))
	seq := expectedSeq
	for _, ev := range evs {
		seq++
		var gseq int64
		var occurred time.Time
		err := tx.QueryRow(ctx, `
			INSERT INTO event_log
			  (global_seq, tenant_id, stream_id, stream_seq, type, schema_ver, payload, actor, correlation_id, causation_id)
			VALUES
			  (nextval('event_global_seq'), $1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING global_seq, occurred_at`,
			tenantID, streamID, seq, ev.Type, ev.SchemaVer, string(ev.Payload), ev.Actor, ev.CorrelationID, ev.CausationID,
		).Scan(&gseq, &occurred)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, fmt.Errorf("%w: stream=%s seq=%d", ErrConcurrencyConflict, streamID, seq)
			}
			return nil, err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO outbox (global_seq) VALUES ($1)", gseq); err != nil {
			return nil, err
		}
		out = append(out, Committed{
			GlobalSeq: gseq, TenantID: tenantID, StreamID: streamID, StreamSeq: seq,
			Type: ev.Type, SchemaVer: ev.SchemaVer, Payload: ev.Payload, Actor: ev.Actor,
			CorrelationID: ev.CorrelationID, CausationID: ev.CausationID, OccurredAt: occurred,
		})
	}
	return out, nil
}

// LoadStream returns one tenant's events of a stream ordered by stream_seq. The
// last element's StreamSeq is the expectedSeq for the next Append (0 if empty).
func (s *Store) LoadStream(ctx context.Context, tx pgx.Tx, tenantID, streamID string) ([]Committed, error) {
	rows, err := tx.Query(ctx, `
		SELECT global_seq, tenant_id, stream_id, stream_seq, type, schema_ver, payload, actor, correlation_id, causation_id, occurred_at
		FROM event_log WHERE tenant_id = $1 AND stream_id = $2 ORDER BY stream_seq`, tenantID, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Committed
	for rows.Next() {
		var c Committed
		if err := rows.Scan(&c.GlobalSeq, &c.TenantID, &c.StreamID, &c.StreamSeq, &c.Type, &c.SchemaVer,
			&c.Payload, &c.Actor, &c.CorrelationID, &c.CausationID, &c.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
