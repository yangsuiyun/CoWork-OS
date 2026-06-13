package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coworkos/cowork-os/v2/server/internal/cap"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/coworkos/cowork-os/v2/server/internal/realtime"
	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func newActionServer(t *testing.T) (e *echo.Echo, issuer *cap.Issuer, rev *cap.RevocationStore, jwtTok, taskID, tenant string) {
	t.Helper()
	dsn := os.Getenv("COWORK_DATABASE_URL")
	if dsn == "" {
		t.Skip("COWORK_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	issuer = cap.NewIssuer(testSecret)
	rev = cap.NewRevocationStore(pool)
	e = echo.New()
	svc := app.New(pool)
	guard := cap.NewGuard(cap.NewVerifier(issuer, rev), cap.NewHookPipeline(nil, nil))
	Register(e, svc, realtime.NewHub(pool), guard, testSecret)

	tenant = fmt.Sprintf("act-%d", time.Now().UnixNano())
	jwtTok = token(t, tenant, "u")
	// Seed a task so RequestApproval (ask path) has a live aggregate.
	taskID = fmt.Sprintf("act-task-%d", time.Now().UnixNano())
	mustPost(t, e, jwtTok, "/v1/commands",
		fmt.Sprintf(`{"type":"CreateTask","payload":{"taskId":%q,"workspaceId":"w","canonicalPrompt":"p","risk":"low"}}`, taskID))
	return e, issuer, rev, jwtTok, taskID, tenant
}

func mustPost(t *testing.T, e *echo.Echo, jwtTok, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtTok)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestActionAllowAskDeny(t *testing.T) {
	e, _, _, jwtTok, taskID, _ := newActionServer(t)

	// allow: fs.read in scope -> R1.
	rec, out := mustPost(t, e, jwtTok, "/v1/actions",
		fmt.Sprintf(`{"taskId":%q,"resource":"fs.read","inScope":true}`, taskID))
	if rec.Code != http.StatusOK || out["decision"] != "allow" || out["ruleId"] != "R1" {
		t.Fatalf("allow: code=%d out=%v", rec.Code, out)
	}

	// ask: fs.write in scope high risk -> R3 -> emits ApprovalRequested.
	rec, out = mustPost(t, e, jwtTok, "/v1/actions",
		fmt.Sprintf(`{"taskId":%q,"resource":"fs.write","inScope":true,"risk":"high"}`, taskID))
	if rec.Code != http.StatusOK || out["decision"] != "ask" || out["ruleId"] != "R3" {
		t.Fatalf("ask: code=%d out=%v", rec.Code, out)
	}
	evs, _ := out["events"].([]any)
	if len(evs) != 1 {
		t.Fatalf("ask should emit ApprovalRequested, got %v", out)
	}

	// deny: tool without capability -> R7 permission_denied.
	rec, out = mustPost(t, e, jwtTok, "/v1/actions",
		fmt.Sprintf(`{"taskId":%q,"resource":"tool:search"}`, taskID))
	if rec.Code != http.StatusForbidden || out["code"] != "permission_denied" {
		t.Fatalf("deny: code=%d out=%v", rec.Code, out)
	}
}

func TestActionCapabilityAllowThenRevoke(t *testing.T) {
	e, issuer, rev, jwtTok, taskID, tenant := newActionServer(t)

	c := cap.Capability{
		TokenId: fmt.Sprintf("cap-%d", time.Now().UnixNano()), Version: 1,
		Subject: "task:" + taskID, Resource: "tool:search",
		Scope:     contracts.CapabilitySchemaJsonScope{WorkspaceId: "w"},
		Revocable: true,
	}
	capTok, err := issuer.Mint(c)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	// With a valid tool:search capability -> R6 allow.
	rec, out := mustPost(t, e, jwtTok, "/v1/actions",
		fmt.Sprintf(`{"taskId":%q,"resource":"tool:search","capabilityToken":%q}`, taskID, capTok))
	if rec.Code != http.StatusOK || out["decision"] != "allow" || out["ruleId"] != "R6" {
		t.Fatalf("cap allow: code=%d out=%v", rec.Code, out)
	}

	// Revoke -> use-time check denies with capability_denied.
	if err := rev.Revoke(context.Background(), tenant, c.TokenId, c.Version, "test"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	rec, out = mustPost(t, e, jwtTok, "/v1/actions",
		fmt.Sprintf(`{"taskId":%q,"resource":"tool:search","capabilityToken":%q}`, taskID, capTok))
	if rec.Code != http.StatusForbidden || out["code"] != "capability_denied" {
		t.Fatalf("revoked cap: code=%d out=%v", rec.Code, out)
	}
}
