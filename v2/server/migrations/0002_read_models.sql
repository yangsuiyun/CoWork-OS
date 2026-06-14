-- +goose Up
-- +goose StatementBegin

-- Read models are projections written ONLY by deterministic projectors that
-- consume event_log. They can be DROPped and rebuilt by replay (spec 8.2/8.3).
-- Rebuild uses a shadow table + swap to avoid downtime (spec review fix F).

CREATE TABLE rm_tasks (
  id             TEXT        NOT NULL,
  tenant_id      TEXT        NOT NULL,
  workspace_id   TEXT        NOT NULL,
  parent_task_id TEXT,
  status         TEXT        NOT NULL,                  -- derived from events, never written directly
  title          TEXT,
  risk           TEXT        NOT NULL,
  origin         TEXT        NOT NULL,
  updated_seq    BIGINT      NOT NULL,
  updated_at     TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX rm_tasks_ws_idx ON rm_tasks (tenant_id, workspace_id, status);

CREATE TABLE rm_timeline (
  tenant_id   TEXT        NOT NULL,                      -- carried for direct RLS, not via join
  task_id     TEXT        NOT NULL,
  seq         BIGINT      NOT NULL,
  kind        TEXT        NOT NULL,
  summary     TEXT,
  occurred_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, task_id, seq)
);

CREATE TABLE rm_artifacts (
  id         TEXT        NOT NULL,
  tenant_id  TEXT        NOT NULL,                       -- carried for direct RLS, not via join
  task_id    TEXT        NOT NULL,
  path       TEXT        NOT NULL,
  sha256     TEXT        NOT NULL,
  mime       TEXT        NOT NULL,
  size       BIGINT      NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, id)
);

-- pgvector. Embeddings are computed ONCE at event time and carried in the event
-- payload; the projector only copies them in. Projectors stay deterministic and
-- never call a model during replay (spec review fix A).
CREATE EXTENSION IF NOT EXISTS vector;
-- Decision: VECTOR(1536) targets OpenAI text-embedding-3-small as the default.
-- Changing the embedding model requires a new migration (dimension is fixed per
-- column). If multiple models must coexist, split per-model tables/columns.
CREATE TABLE rm_memory_vec (
  id           TEXT        NOT NULL,
  tenant_id    TEXT        NOT NULL,
  scope        TEXT        NOT NULL,
  kind         TEXT        NOT NULL,
  embedding    VECTOR(1536),
  text         TEXT        NOT NULL,
  source_event BIGINT      NOT NULL,
  PRIMARY KEY (tenant_id, id)
);

-- All read models enforce tenant isolation in-DB (USING gates reads,
-- WITH CHECK gates projector writes).
ALTER TABLE rm_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_tasks_tenant_isolation ON rm_tasks
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE rm_timeline ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_timeline FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_timeline_tenant_isolation ON rm_timeline
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE rm_artifacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_artifacts FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_artifacts_tenant_isolation ON rm_artifacts
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE rm_memory_vec ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_memory_vec FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_memory_vec_tenant_isolation ON rm_memory_vec
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rm_memory_vec;
DROP TABLE IF EXISTS rm_artifacts;
DROP TABLE IF EXISTS rm_timeline;
DROP TABLE IF EXISTS rm_tasks;
-- +goose StatementEnd
