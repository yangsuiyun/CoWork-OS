-- +goose Up
-- +goose StatementBegin

-- SkillCandidate read model (spec 13.2, Review-First self-learning). Projected
-- from SkillCandidateProposed / Published / Rejected on skillcandidate:<id>.
-- A candidate is only ever a candidate until a human review publishes it.
CREATE TABLE rm_skill_candidates (
  id             TEXT        NOT NULL,                  -- candidateId
  tenant_id      TEXT        NOT NULL,
  name           TEXT        NOT NULL,
  source_task_id TEXT,
  summary        TEXT,
  status         TEXT        NOT NULL,                  -- proposed|published|rejected
  reviewed_by    TEXT,
  updated_seq    BIGINT      NOT NULL,
  updated_at     TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, id)
);
CREATE INDEX rm_skill_candidates_status_idx ON rm_skill_candidates (tenant_id, status);

ALTER TABLE rm_skill_candidates ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_skill_candidates FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_skill_candidates_tenant_isolation ON rm_skill_candidates
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- Least-privilege grants co-located with the table (SSOT for its perms).
GRANT SELECT ON rm_skill_candidates TO cowork_app;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON rm_skill_candidates TO cowork_projector;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rm_skill_candidates;
-- +goose StatementEnd
