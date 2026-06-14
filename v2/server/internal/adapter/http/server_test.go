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
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func testGuard(pool *pgxpool.Pool) *cap.Guard {
	verifier := cap.NewVerifier(cap.NewIssuer(testSecret), cap.NewRevocationStore(pool))
	return cap.NewGuard(verifier, cap.NewHookPipeline(nil, nil))
}

const testSecret = "test-secret"

func token(t *testing.T, tenant, sub string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tid": tenant, "sub": sub, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newServer(t *testing.T) (*echo.Echo, string) {
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
	e := echo.New()
	Register(e, app.New(pool), realtime.NewHub(pool), testGuard(pool), testSecret)
	return e, token(t, fmt.Sprintf("tenant-%d", time.Now().UnixNano()), "user-1")
}

func do(t *testing.T, e *echo.Echo, jwtTok, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/commands", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtTok)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestUnauthorized(t *testing.T) {
	e, _ := newServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/commands", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestCommandLifecycleOverHTTP(t *testing.T) {
	e, jwtTok := newServer(t)
	taskID := fmt.Sprintf("t-%d", time.Now().UnixNano())

	rec, out := do(t, e, jwtTok, fmt.Sprintf(`{"type":"CreateTask","payload":{"taskId":%q,"workspaceId":"w1","canonicalPrompt":"do x","risk":"low"}}`, taskID))
	if rec.Code != http.StatusOK {
		t.Fatalf("create want 200 got %d: %v", rec.Code, out)
	}
	evs, _ := out["events"].([]any)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %v", out)
	}

	rec, out = do(t, e, jwtTok, fmt.Sprintf(`{"type":"StartTurn","payload":{"taskId":%q}}`, taskID))
	if rec.Code != http.StatusOK {
		t.Fatalf("startTurn want 200 got %d: %v", rec.Code, out)
	}

	rec, _ = do(t, e, jwtTok, fmt.Sprintf(`{"type":"CompleteTask","payload":{"taskId":%q}}`, taskID))
	if rec.Code != http.StatusOK {
		t.Fatalf("complete want 200 got %d", rec.Code)
	}

	// INV-2: a turn after completion is rejected (422 invariant_violated).
	rec, out = do(t, e, jwtTok, fmt.Sprintf(`{"type":"StartTurn","payload":{"taskId":%q}}`, taskID))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("post-terminal turn want 422 got %d: %v", rec.Code, out)
	}
	if out["code"] != "invariant_violated" {
		t.Fatalf("want invariant_violated, got %v", out["code"])
	}
}

func TestWorkspaceCommandsOverHTTP(t *testing.T) {
	e, jwtTok := newServer(t)
	wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

	rec, out := do(t, e, jwtTok, fmt.Sprintf(`{"type":"CreateWorkspace","payload":{"workspaceId":%q,"name":"Default"}}`, wsID))
	if rec.Code != http.StatusOK {
		t.Fatalf("create workspace want 200 got %d: %v", rec.Code, out)
	}
	if evs, _ := out["events"].([]any); len(evs) != 1 {
		t.Fatalf("expected 1 event, got %v", out)
	}

	rec, out = do(t, e, jwtTok, fmt.Sprintf(`{"type":"UpdatePermissions","payload":{"workspaceId":%q,"permissions":{"paths":["/repo"]}}}`, wsID))
	if rec.Code != http.StatusOK {
		t.Fatalf("update permissions want 200 got %d: %v", rec.Code, out)
	}

	// Updating a non-existent workspace is an invariant violation (422).
	rec, out = do(t, e, jwtTok, `{"type":"UpdatePermissions","payload":{"workspaceId":"nope","permissions":{}}}`)
	if rec.Code != http.StatusUnprocessableEntity || out["code"] != "invariant_violated" {
		t.Fatalf("update missing want 422 invariant_violated, got %d: %v", rec.Code, out)
	}
}

func TestCommandIdempotencyAndExpectedSeq(t *testing.T) {
	e, jwtTok := newServer(t)
	taskID := fmt.Sprintf("idem-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"type":"CreateTask","idempotencyKey":"k1","expectedStreamSeq":0,"payload":{"taskId":%q,"workspaceId":"w1","canonicalPrompt":"do x","risk":"low"}}`, taskID)

	rec, out := do(t, e, jwtTok, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create want 200 got %d: %v", rec.Code, out)
	}
	firstEvents, _ := out["events"].([]any)
	if len(firstEvents) != 1 {
		t.Fatalf("expected first event, got %v", out)
	}

	rec, out = do(t, e, jwtTok, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("idempotent retry want 200 got %d: %v", rec.Code, out)
	}
	retryEvents, _ := out["events"].([]any)
	if len(retryEvents) != 1 || retryEvents[0].(map[string]any)["globalSeq"] != firstEvents[0].(map[string]any)["globalSeq"] {
		t.Fatalf("retry must return original event, got %v", out)
	}

	rec, out = do(t, e, jwtTok, fmt.Sprintf(`{"type":"StartTurn","idempotencyKey":"k1","payload":{"taskId":%q}}`, taskID))
	if rec.Code != http.StatusConflict || out["code"] != "idempotency_conflict" {
		t.Fatalf("key reuse want 409 idempotency_conflict, got %d: %v", rec.Code, out)
	}

	rec, out = do(t, e, jwtTok, fmt.Sprintf(`{"type":"StartTurn","expectedStreamSeq":0,"payload":{"taskId":%q}}`, taskID))
	if rec.Code != http.StatusConflict || out["code"] != "concurrency_conflict" {
		t.Fatalf("stale expected seq want 409 concurrency_conflict, got %d: %v", rec.Code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	e, jwtTok := newServer(t)
	rec, out := do(t, e, jwtTok, `{"type":"Frobnicate","payload":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d: %v", rec.Code, out)
	}
}
