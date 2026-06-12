// Package httpapi exposes the kernel over HTTP: command dispatch and read-model
// queries, behind JWT auth + tenant scoping. It depends only on the kernel app
// service (boundary direction: adapter -> kernel).
package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// Register mounts the /v1 command and query routes with auth middleware.
func Register(e *echo.Echo, svc *app.Service, jwtSecret string) {
	g := e.Group("/v1", authMiddleware(jwtSecret))
	g.POST("/commands", dispatchCommand(svc))
	g.GET("/query/:name", runQuery(svc))
}

// authMiddleware verifies a short-lived HS256 JWT and extracts tenant/actor.
func authMiddleware(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := c.Request().Header.Get("Authorization")
			tok := strings.TrimPrefix(raw, "Bearer ")
			if tok == raw || tok == "" {
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
			c.Set("tenant", tenant)
			c.Set("actor", actor)
			return next(c)
		}
	}
}

type commandReq struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type eventDTO struct {
	GlobalSeq  int64           `json:"globalSeq"`
	StreamID   string          `json:"streamId"`
	StreamSeq  int64           `json:"streamSeq"`
	Type       string          `json:"type"`
	SchemaVer  int             `json:"schemaVer"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt string          `json:"occurredAt"`
}

func dispatchCommand(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req commandReq
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		tenant, _ := c.Get("tenant").(string)
		actor, _ := c.Get("actor").(string)

		committed, err := svc.Handle(c.Request().Context(), tenant, actor, req.Type, req.Payload)
		if err != nil {
			code, category := app.ErrorCode(err)
			return c.JSON(statusFor(category), map[string]any{"code": code, "message": err.Error()})
		}

		out := make([]eventDTO, 0, len(committed))
		for _, e := range committed {
			out = append(out, eventDTO{
				GlobalSeq: e.GlobalSeq, StreamID: e.StreamID, StreamSeq: e.StreamSeq,
				Type: e.Type, SchemaVer: e.SchemaVer, Payload: json.RawMessage(e.Payload),
				OccurredAt: e.OccurredAt.Format("2006-01-02T15:04:05.000Z07:00"),
			})
		}
		return c.JSON(http.StatusOK, map[string]any{"events": out})
	}
}

func runQuery(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		tenant, _ := c.Get("tenant").(string)
		switch c.Param("name") {
		case "tasks":
			items, err := svc.QueryTasks(c.Request().Context(), tenant, 50)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]any{"code": "internal", "message": err.Error()})
			}
			return c.JSON(http.StatusOK, map[string]any{"items": items})
		default:
			return echo.NewHTTPError(http.StatusNotFound, "unknown query")
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
