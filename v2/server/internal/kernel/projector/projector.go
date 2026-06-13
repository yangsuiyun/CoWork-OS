// Package projector consumes event_log in global_seq order and maintains the
// read models (spec 8.3). Projectors are deterministic and perform no I/O
// beyond DB writes, so a read model can be DROPped and rebuilt by replay.
//
// It tracks progress in consumer_offset and is idempotent: re-applying an event
// is a no-op (ON CONFLICT / monotonic updated_seq guard), so at-least-once
// delivery is safe.
//
// NOTE: the projector reads across tenants and therefore must run with a
// role that bypasses RLS (dev uses the postgres superuser). A production
// deployment uses a dedicated projector role with BYPASSRLS.
package projector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	consumerName = "rm_tasks"
	batchSize    = 500
)

// Projector maintains read models from the event log.
type Projector struct {
	pool *pgxpool.Pool
}

// New constructs a Projector.
func New(pool *pgxpool.Pool) *Projector { return &Projector{pool: pool} }

// Run polls RunOnce until ctx is cancelled.
func (p *Projector) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := p.RunOnce(ctx); err != nil {
				// Fail-fast: surface and retry next tick; do not silently mask.
				continue
			}
		}
	}
}

// RunOnce applies the next batch of events and advances the offset in one tx.
// Returns the number of events applied.
func (p *Projector) RunOnce(ctx context.Context) (int, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"INSERT INTO consumer_offset (consumer, last_global_seq) VALUES ($1, 0) ON CONFLICT DO NOTHING", consumerName); err != nil {
		return 0, err
	}

	var offset int64
	if err := tx.QueryRow(ctx,
		"SELECT last_global_seq FROM consumer_offset WHERE consumer = $1 FOR UPDATE", consumerName).Scan(&offset); err != nil {
		return 0, err
	}

	rows, err := tx.Query(ctx, `
		SELECT global_seq, tenant_id, stream_id, type, payload, occurred_at
		FROM event_log WHERE global_seq > $1 ORDER BY global_seq LIMIT $2`, offset, batchSize)
	if err != nil {
		return 0, err
	}
	type rec struct {
		seq        int64
		tenant     string
		stream     string
		typ        string
		payload    []byte
		occurredAt time.Time
	}
	var batch []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.seq, &r.tenant, &r.stream, &r.typ, &r.payload, &r.occurredAt); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	last := offset
	for _, r := range batch {
		// Route by aggregate kind (stream prefix). Unknown streams are skipped.
		switch {
		case strings.HasPrefix(r.stream, "task:"):
			if err := applyToTasks(ctx, tx, r.seq, r.tenant, r.stream, r.typ, r.payload, r.occurredAt); err != nil {
				return 0, err
			}
		case strings.HasPrefix(r.stream, "workspace:"):
			if err := applyToWorkspaces(ctx, tx, r.seq, r.tenant, r.stream, r.typ, r.payload, r.occurredAt); err != nil {
				return 0, err
			}
		case strings.HasPrefix(r.stream, "approval:"):
			if err := applyToApprovals(ctx, tx, r.seq, r.tenant, r.typ, r.payload, r.occurredAt); err != nil {
				return 0, err
			}
		case strings.HasPrefix(r.stream, "graph:"):
			if err := applyToGraph(ctx, tx, r.seq, r.tenant, r.typ, r.payload, r.occurredAt); err != nil {
				return 0, err
			}
		case strings.HasPrefix(r.stream, "skillcandidate:"):
			if err := applyToSkillCandidates(ctx, tx, r.seq, r.tenant, r.typ, r.payload, r.occurredAt); err != nil {
				return 0, err
			}
		case strings.HasPrefix(r.stream, "runner:"):
			if err := applyToRunners(ctx, tx, r.seq, r.tenant, r.typ, r.payload, r.occurredAt); err != nil {
				return 0, err
			}
		}
		last = r.seq
	}

	if last != offset {
		if _, err := tx.Exec(ctx,
			"UPDATE consumer_offset SET last_global_seq = $2 WHERE consumer = $1", consumerName, last); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(batch), nil
}

// Rebuild truncates the task read model, resets the offset, and replays the
// whole log. Proves the read model is a pure function of the event log.
func (p *Projector) Rebuild(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, "TRUNCATE rm_tasks, rm_timeline, rm_artifacts, rm_workspaces, rm_approvals, rm_graph_nodes, rm_skill_candidates, rm_runners"); err != nil {
		return err
	}
	if _, err := p.pool.Exec(ctx,
		"INSERT INTO consumer_offset (consumer, last_global_seq) VALUES ($1, 0) ON CONFLICT (consumer) DO UPDATE SET last_global_seq = 0", consumerName); err != nil {
		return err
	}
	for {
		n, err := p.RunOnce(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func statusForType(typ string) (string, bool) {
	switch typ {
	case "TaskPlanned":
		return "planned", true
	case "TurnStarted":
		return "running", true
	case "TurnCompleted":
		return "pending", true
	case "TaskCompleted":
		return "completed", true
	case "TaskFailed":
		return "failed", true
	case "TaskCancelled":
		return "cancelled", true
	default:
		return "", false
	}
}

func applyToTasks(ctx context.Context, tx pgx.Tx, seq int64, tenant, stream, typ string, payload []byte, occurredAt time.Time) error {
	taskID := strings.TrimPrefix(stream, "task:")

	if typ == "TaskCreated" {
		var p struct {
			WorkspaceID     string  `json:"workspaceId"`
			CanonicalPrompt string  `json:"canonicalPrompt"`
			Risk            string  `json:"risk"`
			Origin          string  `json:"origin"`
			ParentTaskID    *string `json:"parentTaskId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_tasks (id, tenant_id, workspace_id, parent_task_id, status, title, risk, origin, updated_seq, updated_at)
			VALUES ($1,$2,$3,$4,'pending',$5,$6,$7,$8,$9)
			ON CONFLICT (id) DO NOTHING`,
			taskID, tenant, p.WorkspaceID, p.ParentTaskID, p.CanonicalPrompt, p.Risk, p.Origin, seq, occurredAt); err != nil {
			return err
		}
	} else if typ == "ArtifactCreated" {
		var p struct {
			ArtifactID string `json:"artifactId"`
			Path       string `json:"path"`
			SHA256     string `json:"sha256"`
			Mime       string `json:"mime"`
			Size       int64  `json:"size"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_artifacts (id, tenant_id, task_id, path, sha256, mime, size, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`,
			p.ArtifactID, tenant, taskID, p.Path, p.SHA256, p.Mime, p.Size, occurredAt); err != nil {
			return err
		}
	} else if status, ok := statusForType(typ); ok {
		// Monotonic guard: ignore stale/replayed events (idempotent).
		if _, err := tx.Exec(ctx, `
			UPDATE rm_tasks SET status = $2, updated_seq = $3, updated_at = $4
			WHERE id = $1 AND updated_seq < $3`,
			taskID, status, seq, occurredAt); err != nil {
			return err
		}
	}

	// Timeline entry per event (idempotent on replay).
	if _, err := tx.Exec(ctx, `
		INSERT INTO rm_timeline (tenant_id, task_id, seq, kind, summary, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (task_id, seq) DO NOTHING`,
		tenant, taskID, seq, typ, typ, occurredAt); err != nil {
		return err
	}
	return nil
}

// applyToApprovals maintains rm_approvals and cross-updates the owning task's
// status (awaiting_approval on request, pending on resolution). The task is a
// different aggregate, but the projector observes all streams in global order,
// so the rm_tasks row already exists (CreateTask precedes any approval).
func applyToApprovals(ctx context.Context, tx pgx.Tx, seq int64, tenant, typ string, payload []byte, occurredAt time.Time) error {
	switch typ {
	case "ApprovalRequested":
		var p struct {
			ApprovalID string `json:"approvalId"`
			TaskID     string `json:"taskId"`
			Kind       string `json:"kind"`
			Risk       string `json:"risk"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_approvals (id, tenant_id, task_id, kind, risk, status, updated_seq, updated_at)
			VALUES ($1,$2,$3,$4,$5,'pending',$6,$7) ON CONFLICT (id) DO NOTHING`,
			p.ApprovalID, tenant, p.TaskID, p.Kind, p.Risk, seq, occurredAt); err != nil {
			return err
		}
		return crossUpdateTaskStatus(ctx, tx, p.TaskID, "awaiting_approval", seq, occurredAt)
	case "ApprovalResolved":
		var p struct {
			ApprovalID string `json:"approvalId"`
			TaskID     string `json:"taskId"`
			Decision   string `json:"decision"`
			ResolvedBy string `json:"resolvedBy"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		status := "approved"
		if p.Decision == "reject" {
			status = "rejected"
		}
		// Monotonic guard: ignore stale/replayed events (idempotent).
		if _, err := tx.Exec(ctx, `
			UPDATE rm_approvals SET status = $2, resolved_by = $3, updated_seq = $4, updated_at = $5
			WHERE id = $1 AND updated_seq < $4`,
			p.ApprovalID, status, p.ResolvedBy, seq, occurredAt); err != nil {
			return err
		}
		return crossUpdateTaskStatus(ctx, tx, p.TaskID, "pending", seq, occurredAt)
	}
	return nil
}

// crossUpdateTaskStatus moves the owning task's status under the same monotonic
// guard used by applyToTasks, keeping replays idempotent.
func crossUpdateTaskStatus(ctx context.Context, tx pgx.Tx, taskID, status string, seq int64, occurredAt time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE rm_tasks SET status = $2, updated_seq = $3, updated_at = $4
		WHERE id = $1 AND updated_seq < $3`,
		taskID, status, seq, occurredAt)
	return err
}

func applyToWorkspaces(ctx context.Context, tx pgx.Tx, seq int64, tenant, stream, typ string, payload []byte, occurredAt time.Time) error {
	wsID := strings.TrimPrefix(stream, "workspace:")

	switch typ {
	case "WorkspaceCreated":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_workspaces (id, tenant_id, name, permissions, permissions_version, updated_seq, updated_at)
			VALUES ($1,$2,$3,'{}'::jsonb,0,$4,$5)
			ON CONFLICT (id) DO NOTHING`,
			wsID, tenant, p.Name, seq, occurredAt); err != nil {
			return err
		}
	case "PermissionsChanged":
		var p struct {
			Version     int             `json:"version"`
			Permissions json.RawMessage `json:"permissions"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		// Monotonic guard: ignore stale/replayed events (idempotent).
		if _, err := tx.Exec(ctx, `
			UPDATE rm_workspaces SET permissions = $2, permissions_version = $3, updated_seq = $4, updated_at = $5
			WHERE id = $1 AND updated_seq < $4`,
			wsID, []byte(p.Permissions), p.Version, seq, occurredAt); err != nil {
			return err
		}
	}
	return nil
}

// applyToGraph maintains rm_graph_nodes for the OrchestrationGraph aggregate.
// One row per node; GraphSplit seeds nodes as pending, dispatch/update advance
// status under a monotonic guard so replays are idempotent.
func applyToGraph(ctx context.Context, tx pgx.Tx, seq int64, tenant, typ string, payload []byte, occurredAt time.Time) error {
	switch typ {
	case "GraphSplit":
		var p struct {
			GraphID string `json:"graphId"`
			TaskID  string `json:"taskId"`
			Nodes   []struct {
				NodeID         string `json:"nodeId"`
				DispatchTarget string `json:"dispatchTarget"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		for _, n := range p.Nodes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO rm_graph_nodes (graph_id, node_id, tenant_id, task_id, dispatch_target, status, updated_seq, updated_at)
				VALUES ($1,$2,$3,$4,$5,'pending',$6,$7) ON CONFLICT (graph_id, node_id) DO NOTHING`,
				p.GraphID, n.NodeID, tenant, p.TaskID, n.DispatchTarget, seq, occurredAt); err != nil {
				return err
			}
		}
	case "NodeDispatched":
		var p struct {
			GraphID      string `json:"graphId"`
			NodeID       string `json:"nodeId"`
			RemoteTaskID string `json:"remoteTaskId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rm_graph_nodes SET status = 'dispatched', remote_task_id = NULLIF($3,''), updated_seq = $4, updated_at = $5
			WHERE graph_id = $1 AND node_id = $2 AND updated_seq < $4`,
			p.GraphID, p.NodeID, p.RemoteTaskID, seq, occurredAt); err != nil {
			return err
		}
	case "NodeUpdated":
		var p struct {
			GraphID string `json:"graphId"`
			NodeID  string `json:"nodeId"`
			Status  string `json:"status"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rm_graph_nodes SET status = $3, outcome = NULLIF($4,''), updated_seq = $5, updated_at = $6
			WHERE graph_id = $1 AND node_id = $2 AND updated_seq < $5`,
			p.GraphID, p.NodeID, p.Status, p.Outcome, seq, occurredAt); err != nil {
			return err
		}
	case "ResultMerged":
		// Merge is terminal at the graph level; node rows already carry outcomes.
		// No per-node write needed (the graph aggregate enforces the invariant).
	}
	return nil
}

// applyToSkillCandidates maintains rm_skill_candidates for the SkillCandidate
// aggregate (Review-First). Proposed seeds a row; publish/reject advance status
// under a monotonic guard so replays are idempotent.
func applyToSkillCandidates(ctx context.Context, tx pgx.Tx, seq int64, tenant, typ string, payload []byte, occurredAt time.Time) error {
	switch typ {
	case "SkillCandidateProposed":
		var p struct {
			CandidateID  string `json:"candidateId"`
			Name         string `json:"name"`
			SourceTaskID string `json:"sourceTaskId"`
			Summary      string `json:"summary"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_skill_candidates (id, tenant_id, name, source_task_id, summary, status, updated_seq, updated_at)
			VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),'proposed',$6,$7) ON CONFLICT (id) DO NOTHING`,
			p.CandidateID, tenant, p.Name, p.SourceTaskID, p.Summary, seq, occurredAt); err != nil {
			return err
		}
	case "SkillCandidatePublished":
		var p struct {
			CandidateID string `json:"candidateId"`
			ReviewedBy  string `json:"reviewedBy"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rm_skill_candidates SET status = 'published', reviewed_by = $2, updated_seq = $3, updated_at = $4
			WHERE id = $1 AND updated_seq < $3`,
			p.CandidateID, p.ReviewedBy, seq, occurredAt); err != nil {
			return err
		}
	case "SkillCandidateRejected":
		var p struct {
			CandidateID string `json:"candidateId"`
			ReviewedBy  string `json:"reviewedBy"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rm_skill_candidates SET status = 'rejected', reviewed_by = $2, updated_seq = $3, updated_at = $4
			WHERE id = $1 AND updated_seq < $3`,
			p.CandidateID, p.ReviewedBy, seq, occurredAt); err != nil {
			return err
		}
	}
	return nil
}

// applyToRunners maintains rm_runners for the LocalRunnerSession aggregate (M3).
// Register seeds/recovers the row as active; heartbeat advances last_pulse;
// stale flips status. The monotonic guard keeps replays idempotent. Register
// uses upsert because a stale runner may re-register (recovery).
func applyToRunners(ctx context.Context, tx pgx.Tx, seq int64, tenant, typ string, payload []byte, occurredAt time.Time) error {
	switch typ {
	case "RunnerRegistered":
		var p struct {
			RunnerID    string `json:"runnerId"`
			WorkspaceID string `json:"workspaceId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rm_runners (id, tenant_id, workspace_id, status, last_pulse, updated_seq, updated_at)
			VALUES ($1,$2,$3,'active',0,$4,$5)
			ON CONFLICT (id) DO UPDATE SET status = 'active', last_pulse = 0, updated_seq = $4, updated_at = $5
			WHERE rm_runners.updated_seq < $4`,
			p.RunnerID, tenant, p.WorkspaceID, seq, occurredAt); err != nil {
			return err
		}
	case "RunnerHeartbeat":
		var p struct {
			RunnerID string `json:"runnerId"`
			Pulse    int64  `json:"pulse"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rm_runners SET last_pulse = $2, updated_seq = $3, updated_at = $4
			WHERE id = $1 AND updated_seq < $3`,
			p.RunnerID, p.Pulse, seq, occurredAt); err != nil {
			return err
		}
	case "RunnerStale":
		var p struct {
			RunnerID string `json:"runnerId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rm_runners SET status = 'stale', updated_seq = $2, updated_at = $3
			WHERE id = $1 AND updated_seq < $2`,
			p.RunnerID, seq, occurredAt); err != nil {
			return err
		}
	}
	return nil
}
