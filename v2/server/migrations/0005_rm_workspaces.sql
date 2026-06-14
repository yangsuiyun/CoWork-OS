-- +goose Up
-- +goose StatementBegin

-- Workspace read model (spec 7.2). Projected from WorkspaceCreated /
-- PermissionsChanged; rebuildable by replay like every other read model.
CREATE TABLE rm_workspaces (
  id                  TEXT        NOT NULL,
  tenant_id           TEXT        NOT NULL,
  name                TEXT        NOT NULL,
  permissions         JSONB       NOT NULL DEFAULT '{}'::jsonb,
  permissions_version INT         NOT NULL DEFAULT 0,
  updated_seq         BIGINT      NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX rm_workspaces_tenant_idx ON rm_workspaces (tenant_id);

ALTER TABLE rm_workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_workspaces FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_workspaces_tenant_isolation ON rm_workspaces
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- Least-privilege grants co-located with the table (SSOT for its perms):
-- request path reads only; the projector owns writes.
GRANT SELECT ON rm_workspaces TO cowork_app;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON rm_workspaces TO cowork_projector;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rm_workspaces;
-- +goose StatementEnd
