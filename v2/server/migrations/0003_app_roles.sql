-- +goose Up
-- +goose StatementBegin
-- Least-privilege roles so RLS is actually ENFORCED (spec 11.0). The migration
-- owner (postgres) is a superuser and BYPASSES RLS; the request path must run
-- as a non-superuser, non-owner role with no BYPASSRLS so tenant policies apply.
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'cowork_app') THEN
    CREATE ROLE cowork_app LOGIN PASSWORD 'cowork';
  END IF;
  -- Projector is a system consumer that must read ALL tenants from event_log,
  -- so it is granted BYPASSRLS. It never serves client requests.
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'cowork_projector') THEN
    CREATE ROLE cowork_projector LOGIN PASSWORD 'cowork' BYPASSRLS;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
GRANT USAGE ON SCHEMA public TO cowork_app, cowork_projector;

-- Request-path role: append events + read read-models, RLS-scoped per tx.
GRANT SELECT, INSERT ON event_log TO cowork_app;
GRANT SELECT, INSERT ON outbox TO cowork_app;
GRANT SELECT, INSERT ON processed_command TO cowork_app;
GRANT SELECT ON rm_tasks, rm_timeline, rm_artifacts, rm_memory_vec TO cowork_app;
GRANT USAGE, SELECT ON SEQUENCE event_global_seq TO cowork_app;

-- Projector role: read the full log, maintain read models, track offsets.
GRANT SELECT ON event_log TO cowork_projector;
GRANT SELECT, INSERT, UPDATE ON consumer_offset TO cowork_projector;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON rm_tasks, rm_timeline, rm_artifacts, rm_memory_vec TO cowork_projector;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP OWNED BY cowork_app, cowork_projector;
DROP ROLE IF EXISTS cowork_app;
DROP ROLE IF EXISTS cowork_projector;
-- +goose StatementEnd
