package projector_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/projector"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(t *testing.T) (*pgxpool.Pool, *app.Service, *projector.Projector, context.Context) {
	t.Helper()
	dsn := os.Getenv("COWORK_DATABASE_URL")
	if dsn == "" {
		t.Skip("COWORK_DATABASE_URL not set; skipping integration test")
	}
	projDSN := os.Getenv("COWORK_PROJECTOR_DATABASE_URL")
	if projDSN == "" {
		projDSN = dsn
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	projPool, err := pgxpool.New(ctx, projDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(projPool.Close)
	return pool, app.New(pool), projector.New(projPool), ctx
}

// readTask reads a single task's status from rm_tasks (RLS-scoped to tenant).
func readTask(t *testing.T, pool *pgxpool.Pool, ctx context.Context, tenant, id string) (string, int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant)
	var status string
	var seq int64
	err = tx.QueryRow(ctx, "SELECT status, updated_seq FROM rm_tasks WHERE id = $1", id).Scan(&status, &seq)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", 0
		}
		t.Fatal(err)
	}
	return status, seq
}

func TestProjectionAndDeterministicRebuild(t *testing.T) {
	pool, svc, proj, ctx := setup(t)
	tenant := fmt.Sprintf("tenant-%d", os.Getpid())
	id := fmt.Sprintf("task-proj-%d", os.Getpid())

	steps := []string{
		fmt.Sprintf(`{"taskId":%q,"workspaceId":"w","canonicalPrompt":"p","risk":"low"}`, id),
	}
	if _, err := svc.Handle(ctx, tenant, "u", "CreateTask", []byte(steps[0])); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Handle(ctx, tenant, "u", "StartTurn", []byte(fmt.Sprintf(`{"taskId":%q}`, id))); err != nil {
		t.Fatalf("startTurn: %v", err)
	}
	if _, err := svc.Handle(ctx, tenant, "u", "CompleteTask", []byte(fmt.Sprintf(`{"taskId":%q}`, id))); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("project: %v", err)
	}
	status, seq := readTask(t, pool, ctx, tenant, id)
	if status != "completed" {
		t.Fatalf("after projection want completed, got %q", status)
	}

	// Idempotency: a re-run may pick up unrelated events from concurrent test
	// packages (the log is global), but must never change THIS task's state.
	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	status2, seq2 := readTask(t, pool, ctx, tenant, id)
	if status2 != status || seq2 != seq {
		t.Fatalf("idempotency broken: (%s,%d) != (%s,%d)", status2, seq2, status, seq)
	}

	// Determinism: full rebuild from the log yields the identical row.
	if err := proj.Rebuild(ctx); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	status3, seq3 := readTask(t, pool, ctx, tenant, id)
	if status3 != status || seq3 != seq {
		t.Fatalf("rebuild not deterministic: (%s,%d) != (%s,%d)", status3, seq3, status, seq)
	}
}

// TestTenantIsolation proves RLS: a task created under tenant A is invisible to
// a query scoped to tenant B. This only holds when DatabaseURL uses the
// RLS-scoped cowork_app role (a superuser DSN bypasses RLS and this fails).
func TestTenantIsolation(t *testing.T) {
	_, svc, proj, ctx := setup(t)
	tenantA := fmt.Sprintf("A-%d", os.Getpid())
	tenantB := fmt.Sprintf("B-%d", os.Getpid())
	id := fmt.Sprintf("iso-%d", os.Getpid())

	if _, err := svc.Handle(ctx, tenantA, "u", "CreateTask",
		[]byte(fmt.Sprintf(`{"taskId":%q,"workspaceId":"w","canonicalPrompt":"secret","risk":"low"}`, id))); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("project: %v", err)
	}

	aTasks, err := svc.QueryTasks(ctx, tenantA, 100)
	if err != nil {
		t.Fatalf("query A: %v", err)
	}
	if !containsTask(aTasks, id) {
		t.Fatalf("tenant A must see its own task %s", id)
	}

	bTasks, err := svc.QueryTasks(ctx, tenantB, 100)
	if err != nil {
		t.Fatalf("query B: %v", err)
	}
	if containsTask(bTasks, id) {
		t.Fatalf("RLS breach: tenant B can see tenant A's task %s", id)
	}
}

func TestApprovalAndArtifactProjection(t *testing.T) {
	pool, svc, proj, ctx := setup(t)
	tenant := fmt.Sprintf("aa-%d", os.Getpid())
	id := fmt.Sprintf("aa-task-%d", os.Getpid())

	must := func(cmd string, body string) {
		if _, err := svc.Handle(ctx, tenant, "u", cmd, []byte(body)); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
	must("CreateTask", fmt.Sprintf(`{"taskId":%q,"workspaceId":"w","canonicalPrompt":"p","risk":"low"}`, id))
	must("RequestApproval", fmt.Sprintf(`{"taskId":%q,"approvalId":"ap1","kind":"shell","risk":"high"}`, id))
	must("AppendArtifact", fmt.Sprintf(`{"taskId":%q,"artifactId":"art1","path":"/o.txt","sha256":"deadbeef","mime":"text/plain","size":3}`, id))

	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("project: %v", err)
	}

	status, _ := readTask(t, pool, ctx, tenant, id)
	if status != "awaiting_approval" {
		t.Fatalf("want awaiting_approval, got %q", status)
	}

	tx, _ := pool.Begin(ctx)
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant)
	var path string
	if err := tx.QueryRow(ctx, "SELECT path FROM rm_artifacts WHERE id = $1", "art1").Scan(&path); err != nil {
		t.Fatalf("artifact not projected: %v", err)
	}
	if path != "/o.txt" {
		t.Fatalf("unexpected artifact path %q", path)
	}
}

func containsTask(tasks []app.TaskView, id string) bool {
	for _, v := range tasks {
		if v.ID == id {
			return true
		}
	}
	return false
}
