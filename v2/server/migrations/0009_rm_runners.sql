-- +goose Up
-- +goose StatementBegin

-- LocalRunnerSession read model (M3 contract layer, spec 20.4). Projected from
-- RunnerRegistered / RunnerHeartbeat / RunnerStale on the runner:<id> stream.
-- Tracks the latest heartbeat pulse and liveness; the real reverse gRPC/WS
-- tunnel is a separate runtime project.
CREATE TABLE rm_runners (
  id           TEXT        NOT NULL,                    -- runnerId
  tenant_id    TEXT        NOT NULL,
  workspace_id TEXT        NOT NULL,
  status       TEXT        NOT NULL,                    -- active|stale
  last_pulse   BIGINT      NOT NULL DEFAULT 0,
  updated_seq  BIGINT      NOT NULL,
  updated_at   TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX rm_runners_ws_idx ON rm_runners (tenant_id, workspace_id, status);

ALTER TABLE rm_runners ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_runners FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_runners_tenant_isolation ON rm_runners
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- Least-privilege grants co-located with the table (SSOT for its perms).
GRANT SELECT ON rm_runners TO cowork_app;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON rm_runners TO cowork_projector;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rm_runners;
-- +goose StatementEnd
