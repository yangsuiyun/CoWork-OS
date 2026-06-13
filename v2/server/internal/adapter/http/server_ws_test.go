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
	"github.com/coworkos/cowork-os/v2/server/internal/realtime"
	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func TestWebSocketLiveStream(t *testing.T) {
	dsn := os.Getenv("COWORK_DATABASE_URL")
	if dsn == "" {
		t.Skip("COWORK_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	hub := realtime.NewHub(pool)
	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()
	go func() { _ = hub.Run(hubCtx) }()

	e := echo.New()
	Register(e, app.New(pool), hub, testVerifier(pool), testSecret)
	ts := httptest.NewServer(e)
	defer ts.Close()

	tenant := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	jwtTok := token(t, tenant, "u")

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/v1/stream?from=0&token=" + jwtTok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Give the WS handler a moment to register with the hub before we commit.
	time.Sleep(200 * time.Millisecond)

	taskID := fmt.Sprintf("ws-task-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"type":"CreateTask","payload":{"taskId":%q,"workspaceId":"w","canonicalPrompt":"hi","risk":"low"}}`, taskID)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/commands", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+jwtTok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status %d", resp.StatusCode)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws: %v", err)
	}
	var got contracts.CommittedEvent
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "TaskCreated" || got.StreamId != "task:"+taskID {
		t.Fatalf("unexpected event: %+v", got)
	}
}
