-- +goose Up
-- +goose StatementBegin
-- Capability revocation store (spec 11.1). A capability token is checked here
-- on every use; revoking bumps revoked_version so all tokens with
-- version <= revoked_version are denied immediately (in-flight calls are
-- cancelled by the caller on the next use-time check).
CREATE TABLE cap_revocation (
  tenant_id       TEXT        NOT NULL,
  token_id        TEXT        NOT NULL,
  revoked_version INT         NOT NULL,            -- versions <= this are revoked
  reason          TEXT,
  revoked_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, token_id)
);

ALTER TABLE cap_revocation ENABLE ROW LEVEL SECURITY;
ALTER TABLE cap_revocation FORCE ROW LEVEL SECURITY;
CREATE POLICY cap_revocation_tenant_isolation ON cap_revocation
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

GRANT SELECT, INSERT, UPDATE ON cap_revocation TO cowork_app;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS cap_revocation;
-- +goose StatementEnd
