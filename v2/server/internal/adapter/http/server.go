// Package httpapi exposes the kernel over HTTP: command dispatch and read-model
// queries, behind JWT auth + tenant scoping. It depends only on the kernel app
// service (boundary direction: adapter -> kernel).
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/coworkos/cowork-os/v2/server/internal/cap"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/realtime"
	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	// Origin checks are enforced upstream (reverse proxy / CORS); the JWT in the
	// query/header is the actual authorization gate.
	CheckOrigin: func(*http.Request) bool { return true },
}

// Register mounts the /v1 command, query, stream, action, and session routes
// with auth.
func Register(e *echo.Echo, svc *app.Service, hub *realtime.Hub, guard *cap.Guard, jwtSecret string) {
	g := e.Group("/v1", authMiddleware(jwtSecret))
	g.POST("/commands", dispatchCommand(svc))
	g.GET("/query/:name", runQuery(svc))
	g.GET("/stream", streamEvents(svc, hub))
	g.POST("/actions", authorizeAction(svc, guard))
	registerSessions(g, svc, hub)
}

// authMiddleware verifies a short-lived HS256 JWT and extracts tenant/actor.
// The token comes from the Authorization header, or a ?token= query param as a
// fallback for browser WebSocket clients (which cannot set custom headers).
func authMiddleware(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := c.Request().Header.Get("Authorization")
			tok := strings.TrimPrefix(raw, "Bearer ")
			if tok == raw || tok == "" {
				tok = c.QueryParam("token")
			}
			if tok == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}
			claims := jwt.MapClaims{}
			_, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) {
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}
			tenant, _ := claims["tid"].(string)
			if tenant == "" {
				return echo.NewHTTPError(http.StatusForbidden, "missing tenant claim")
			}
			actor, _ := claims["sub"].(string)
			if actor == "" {
				return echo.NewHTTPError(http.StatusForbidden, "missing subject claim")
			}
			c.Set("tenant", tenant)
			c.Set("actor", actor)
			return next(c)
		}
	}
}

func dispatchCommand(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req contracts.Command
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		tenant, _ := c.Get("tenant").(string)
		actor, _ := c.Get("actor").(string)

		payload, err := json.Marshal(req.Payload)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
		}
		opt := app.CommandOptions{ExpectedStreamSeq: req.ExpectedStreamSeq}
		if req.IdempotencyKey != nil {
			opt.IdempotencyKey = *req.IdempotencyKey
		}
		committed, err := svc.Handle(c.Request().Context(), tenant, actor, req.Type, payload, opt)
		if err != nil {
			code, category := app.ErrorCode(err)
			return c.JSON(statusFor(category), contracts.DomainError{Code: code, Message: err.Error()})
		}

		out := make([]contracts.CommittedEvent, 0, len(committed))
		for _, e := range committed {
			out = append(out, toCommittedEvent(e))
		}
		return c.JSON(http.StatusOK, map[string]any{"events": out})
	}
}

// toCommittedEvent maps a stored event to the contract wire type (SSOT).
func toCommittedEvent(e events.Committed) contracts.CommittedEvent {
	var payload map[string]any
	_ = json.Unmarshal(e.Payload, &payload)
	return contracts.CommittedEvent{
		Actor: e.Actor, CausationId: e.CausationID, CorrelationId: e.CorrelationID,
		GlobalSeq: e.GlobalSeq, OccurredAt: e.OccurredAt, Payload: payload,
		SchemaVer: e.SchemaVer, StreamId: e.StreamID, StreamSeq: e.StreamSeq,
		TenantId: e.TenantID, Type: e.Type,
	}
}

func runQuery(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		tenant, _ := c.Get("tenant").(string)
		switch c.Param("name") {
		case "tasks":
			items, err := svc.QueryTasks(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
			}
			return c.JSON(http.StatusOK, contracts.ReadModelPage{Items: toItemMaps(items)})
		case "workspaces":
			items, err := svc.QueryWorkspaces(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
			}
			return c.JSON(http.StatusOK, contracts.ReadModelPage{Items: toItemMaps(items)})
		case "approvals":
			items, err := svc.QueryApprovals(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
			}
			return c.JSON(http.StatusOK, contracts.ReadModelPage{Items: toItemMaps(items)})
		case "graphNodes":
			items, err := svc.QueryGraphNodes(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
			}
			return c.JSON(http.StatusOK, contracts.ReadModelPage{Items: toItemMaps(items)})
		case "skillCandidates":
			items, err := svc.QuerySkillCandidates(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
			}
			return c.JSON(http.StatusOK, contracts.ReadModelPage{Items: toItemMaps(items)})
		case "runners":
			items, err := svc.QueryRunners(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
			}
			return c.JSON(http.StatusOK, contracts.ReadModelPage{Items: toItemMaps(items)})
		default:
			return echo.NewHTTPError(http.StatusNotFound, "unknown query")
		}
	}
}

// toItemMaps renders typed read-model rows as the contract's generic items.
func toItemMaps[T any](rows []T) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		raw, _ := json.Marshal(r)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		out = append(out, m)
	}
	return out
}

// streamEvents upgrades to WebSocket and streams the tenant's events from the
// ?from=<globalSeq> cursor: it backfills missed events, then live-streams as
// the hub pokes it on each new commit. Reads are RLS-scoped to the tenant.
func streamEvents(svc *app.Service, hub *realtime.Hub) echo.HandlerFunc {
	return func(c echo.Context) error {
		tenant, _ := c.Get("tenant").(string)
		from, _ := strconv.ParseInt(c.QueryParam("from"), 10, 64)

		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer ws.Close()

		poke, release := hub.Subscribe()
		defer release()

		ctx, cancel := context.WithCancel(c.Request().Context())
		defer cancel()
		// Detect client close/error by reading; control frames are handled here.
		go func() {
			for {
				if _, _, err := ws.ReadMessage(); err != nil {
					cancel()
					return
				}
			}
		}()

		cursor := from
		drain := func() error {
			for {
				evs, err := svc.EventsSince(ctx, tenant, cursor, 200)
				if err != nil || len(evs) == 0 {
					return err
				}
				for _, e := range evs {
					if err := ws.WriteJSON(toCommittedEvent(e)); err != nil {
						return err
					}
					cursor = e.GlobalSeq
				}
			}
		}

		if err := drain(); err != nil {
			return nil
		}
		for {
			select {
			case <-ctx.Done():
				return nil
			case _, ok := <-poke:
				if !ok {
					return nil
				}
				if err := drain(); err != nil {
					return nil
				}
			}
		}
	}
}

func statusFor(category string) int {
	switch category {
	case "bad_request":
		return http.StatusBadRequest
	case "conflict":
		return http.StatusConflict
	case "unprocessable":
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
