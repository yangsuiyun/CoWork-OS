-- +goose Up
-- +goose StatementBegin

-- Append-only event store. Never UPDATE/DELETE (spec P2, 8.1).
CREATE TABLE event_log (
  global_seq     BIGINT      NOT NULL,                 -- per-shard monotonic VISIBLE order (see note below)
  tenant_id      TEXT        NOT NULL,                  -- multi-tenant isolation (RLS)
  stream_id      TEXT        NOT NULL,                  -- aggregate instance, e.g. task:<id>
  stream_seq     BIGINT      NOT NULL,                  -- per-stream order (optimistic concurrency)
  type           TEXT        NOT NULL,
  schema_ver     INT         NOT NULL,
  payload        JSONB       NOT NULL,
  actor          TEXT        NOT NULL,
  correlation_id TEXT,
  causation_id   TEXT,
  occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (global_seq),
  UNIQUE (tenant_id, stream_id, stream_seq)             -- concurrent write conflict -> retry command
);
CREATE INDEX event_log_tenant_seq_idx ON event_log (tenant_id, global_seq);
CREATE INDEX event_log_stream_idx ON event_log (tenant_id, stream_id, stream_seq);

-- global_seq is assigned by the single writer from this sequence while holding a
-- per-shard advisory lock, so the committed value is monotonically VISIBLE and
-- consumers never skip a "lower-seq-but-later-committed" event (spec 8.1 option 1).
-- Do NOT use BIGSERIAL here: BIGSERIAL allocates pre-commit and can commit out of order.
CREATE SEQUENCE event_global_seq AS BIGINT START 1;

-- Transactional outbox: events that require external side-effect delivery are
-- enqueued in the SAME transaction as the append. Per-consumer progress is
-- tracked in consumer_offset (NOT a single `dispatched` bool, see spec 8.1 fix).
CREATE TABLE outbox (
  global_seq BIGINT NOT NULL REFERENCES event_log (global_seq),
  PRIMARY KEY (global_seq)
);

-- Unified per-consumer cursor for projectors, WS fan-out, and outbox dispatchers.
-- Each reads event_log/outbox WHERE global_seq > last_global_seq, idempotently.
CREATE TABLE consumer_offset (
  consumer        TEXT   NOT NULL,
  last_global_seq BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (consumer)
);

-- Command idempotency: dedup safe client retries and saga requestIds (spec 10).
-- The command handler checks/records the key in the SAME transaction as append,
-- so a retried command returns the prior result instead of re-executing.
CREATE TABLE processed_command (
  tenant_id       TEXT        NOT NULL,
  idempotency_key TEXT        NOT NULL,
  command_type    TEXT        NOT NULL,
  result_seqs     BIGINT[]    NOT NULL,                 -- global_seqs produced by the original run
  processed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key)
);
ALTER TABLE processed_command ENABLE ROW LEVEL SECURITY;
ALTER TABLE processed_command FORCE ROW LEVEL SECURITY;
CREATE POLICY processed_command_tenant_isolation ON processed_command
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- Row-level security: tenant isolation enforced in DB (spec 11.0).
-- The kernel sets `SET LOCAL app.tenant_id = '<tenant>'` per request transaction;
-- a missing/empty setting matches nothing (default-deny, never full-table).
-- USING gates reads; WITH CHECK gates appends (explicit, not relying on the
-- implicit USING-as-check fallback).
ALTER TABLE event_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_log FORCE ROW LEVEL SECURITY;
CREATE POLICY event_log_tenant_isolation ON event_log
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS processed_command;
DROP TABLE IF EXISTS consumer_offset;
DROP TABLE IF EXISTS outbox;
DROP SEQUENCE IF EXISTS event_global_seq;
DROP TABLE IF EXISTS event_log;
-- +goose StatementEnd
