package projector_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestWorkspaceProjection(t *testing.T) {
	pool, svc, proj, ctx := setup(t)
	tenant := fmt.Sprintf("ws-%d", os.Getpid())
	id := fmt.Sprintf("ws-proj-%d", os.Getpid())

	if _, err := svc.Handle(ctx, tenant, "u", "CreateWorkspace",
		[]byte(fmt.Sprintf(`{"workspaceId":%q,"name":"Default"}`, id))); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Handle(ctx, tenant, "u", "UpdatePermissions",
		[]byte(fmt.Sprintf(`{"workspaceId":%q,"permissions":{"paths":["/repo"],"domains":["example.com"]}}`, id))); err != nil {
		t.Fatalf("update: %v", err)
	}

	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("project: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant)

	var name string
	var version int
	var perms []byte
	err = tx.QueryRow(ctx,
		"SELECT name, permissions_version, permissions FROM rm_workspaces WHERE id = $1", id).
		Scan(&name, &version, &perms)
	if err != nil {
		if err == pgx.ErrNoRows {
			t.Fatal("workspace not projected")
		}
		t.Fatal(err)
	}
	if name != "Default" || version != 1 {
		t.Fatalf("want (Default,1), got (%s,%d)", name, version)
	}
	var p struct {
		Paths   []string `json:"paths"`
		Domains []string `json:"domains"`
	}
	if err := json.Unmarshal(perms, &p); err != nil {
		t.Fatalf("permissions json: %v", err)
	}
	if len(p.Paths) != 1 || p.Paths[0] != "/repo" || len(p.Domains) != 1 || p.Domains[0] != "example.com" {
		t.Fatalf("unexpected permissions: %+v", p)
	}
}
