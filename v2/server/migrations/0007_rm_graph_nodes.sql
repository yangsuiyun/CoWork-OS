-- +goose Up
-- +goose StatementBegin

-- OrchestrationGraph read model (spec 12.1). Projected from GraphSplit /
-- NodeDispatched / NodeUpdated / ResultMerged on the graph:<id> stream. One row
-- per node; rebuildable by replay like every other read model.
CREATE TABLE rm_graph_nodes (
  graph_id        TEXT        NOT NULL,
  node_id         TEXT        NOT NULL,
  tenant_id       TEXT        NOT NULL,
  task_id         TEXT        NOT NULL,                  -- owning task
  dispatch_target TEXT        NOT NULL,                  -- local|remote
  remote_task_id  TEXT,                                  -- set when dispatched to remote
  status          TEXT        NOT NULL,                  -- pending|dispatched|done|failed
  outcome         TEXT,
  updated_seq     BIGINT      NOT NULL,
  updated_at      TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (graph_id, node_id)
);
CREATE INDEX rm_graph_nodes_task_idx ON rm_graph_nodes (tenant_id, task_id);

ALTER TABLE rm_graph_nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE rm_graph_nodes FORCE ROW LEVEL SECURITY;
CREATE POLICY rm_graph_nodes_tenant_isolation ON rm_graph_nodes
  USING (tenant_id = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- Least-privilege grants co-located with the table (SSOT for its perms):
-- request path reads only; the projector owns writes.
GRANT SELECT ON rm_graph_nodes TO cowork_app;
GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE ON rm_graph_nodes TO cowork_projector;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rm_graph_nodes;
-- +goose StatementEnd
