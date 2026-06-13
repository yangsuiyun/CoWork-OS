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

func testVerifier(pool *pgxpool.Pool) *cap.Verifier {
	return cap.NewVerifier(cap.NewIssuer(testSecret), cap.NewRevocationStore(pool))
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
	Register(e, app.New(pool), realtime.NewHub(pool), testVerifier(pool), testSecret)
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

func TestUnknownCommand(t *testing.T) {
	e, jwtTok := newServer(t)
	rec, out := do(t, e, jwtTok, `{"type":"Frobnicate","payload":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d: %v", rec.Code, out)
	}
}
