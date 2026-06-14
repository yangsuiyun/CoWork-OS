-- +goose Up
-- +goose StatementBegin

-- ApprovalRequest read model (spec 7.2). Projected from ApprovalRequested /
-- ApprovalResolved on the approval:<id> stream. Carries task_id so the
-- projector can cross-update rm_tasks.status (awaiting_approval / pending).
CREATE TABLE rm_approvals (
  id          TEXT        NOT NULL,                       -- approvalId
  tenant_id   TEXT        NOT NULL,
  task_id     TEXT        NOT NULL,
  kind        TEXT        NOT NULL,                        -- tool|data_export|shell|fs_write|net
  risk        TEXT        NOT NULL,
  status      TEXT        NOT NULL,                        -- pending|approved|rejected
  resolved_by TEXT,
  reason      TEXT,
  updated_seq BIGINT      NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX rm_approvals_task_idx ON rm_approvals (tenant_id, task_id, status);

ALTER TABLE rm_approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_approvals FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_approvals_tenant_isolation ON rm_approvals
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- Least-privilege grants co-located with the table (SSOT for its perms).
GRANT SELECT ON rm_approvals TO cowork_app;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON rm_approvals TO cowork_projector;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rm_approvals;
-- +goose StatementEnd
