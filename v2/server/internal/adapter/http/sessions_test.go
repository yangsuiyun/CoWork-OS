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

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/projector"
	"github.com/coworkos/cowork-os/v2/server/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// newSessionServer wires an echo server plus a projector so session reads
// (which hit the read model) can be advanced deterministically in-test.
func newSessionServer(t *testing.T) (e *echo.Echo, proj *projector.Projector, jwtTok string, ctx context.Context) {
	t.Helper()
	dsn := os.Getenv("COWORK_DATABASE_URL")
	if dsn == "" {
		t.Skip("COWORK_DATABASE_URL not set; skipping integration test")
	}
	projDSN := os.Getenv("COWORK_PROJECTOR_DATABASE_URL")
	if projDSN == "" {
		projDSN = dsn
	}
	ctx = context.Background()
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

	e = echo.New()
	Register(e, app.New(pool), realtime.NewHub(pool), testGuard(pool), testSecret)
	jwtTok = token(t, fmt.Sprintf("sess-%d", time.Now().UnixNano()), "u")
	return e, projector.New(projPool), jwtTok, ctx
}

func TestSessionLifecycle(t *testing.T) {
	e, proj, jwtTok, ctx := newSessionServer(t)

	// Create.
	rec, out := mustPost(t, e, jwtTok, "/v1/sessions", `{"prompt":"do a thing","workspaceId":"w"}`)
	if rec.Code != http.StatusCreated || out["status"] != "pending" {
		t.Fatalf("create: code=%d out=%v", rec.Code, out)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("missing session id: %v", out)
	}

	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("project: %v", err)
	}

	// Get -> pending after CreateTask.
	rec, out = mustGet(t, e, jwtTok, "/v1/sessions/"+id)
	if rec.Code != http.StatusOK || out["status"] != "pending" {
		t.Fatalf("get: code=%d out=%v", rec.Code, out)
	}

	// Cancel -> emits TaskCancelled.
	rec, out = mustPost(t, e, jwtTok, "/v1/sessions/"+id+"/cancel", `{}`)
	evs, _ := out["events"].([]any)
	if rec.Code != http.StatusOK || len(evs) == 0 {
		t.Fatalf("cancel: code=%d out=%v", rec.Code, out)
	}

	if _, err := proj.RunOnce(ctx); err != nil {
		t.Fatalf("project after cancel: %v", err)
	}

	// Get -> cancelled.
	rec, out = mustGet(t, e, jwtTok, "/v1/sessions/"+id)
	if rec.Code != http.StatusOK || out["status"] != "cancelled" {
		t.Fatalf("get after cancel: code=%d out=%v", rec.Code, out)
	}

	// Unknown session -> 404.
	rec, _ = mustGet(t, e, jwtTok, "/v1/sessions/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session want 404, got %d", rec.Code)
	}
}

func TestSessionEventsSSE(t *testing.T) {
	e, _, jwtTok, _ := newSessionServer(t)

	rec, out := mustPost(t, e, jwtTok, "/v1/sessions", `{"prompt":"sse please"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: code=%d out=%v", rec.Code, out)
	}
	id, _ := out["id"].(string)

	// The SSE handler backfills then blocks; cancel the request context shortly
	// after so the backfilled frames are flushed and the handler returns.
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+id+"/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+jwtTok)
	srec := httptest.NewRecorder()
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	e.ServeHTTP(srec, req)

	body := srec.Body.String()
	if !strings.Contains(body, "data:") || !strings.Contains(body, "TaskCreated") {
		t.Fatalf("SSE stream missing TaskCreated frame: %q", body)
	}
}

func mustGet(t *testing.T, e *echo.Echo, jwtTok, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+jwtTok)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}
