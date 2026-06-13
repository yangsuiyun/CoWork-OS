package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/coworkos/cowork-os/v2/server/internal/realtime"
	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// registerSessions mounts the external programmatic API (/v1/sessions). A
// managed session is a thin facade over a Task aggregate: the session id IS the
// task id, so create/get/cancel map to task commands and the read model.
func registerSessions(g *echo.Group, svc *app.Service, hub *realtime.Hub) {
	g.POST("/sessions", createSession(svc))
	g.GET("/sessions/:id", getSession(svc))
	g.GET("/sessions/:id/events", streamSessionEvents(svc, hub))
	g.POST("/sessions/:id/cancel", cancelSession(svc))
}

func createSession(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req contracts.CreateManagedSessionJSONBody
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Prompt) == "" {
			return c.JSON(http.StatusBadRequest, contracts.DomainError{Code: "invalid_request", Message: "prompt is required"})
		}
		tenant, _ := c.Get("tenant").(string)
		actor, _ := c.Get("actor").(string)

		workspaceID := ""
		if req.WorkspaceId != nil {
			workspaceID = *req.WorkspaceId
		}
		taskID := uuid.NewString()
		payload, _ := json.Marshal(map[string]any{
			"taskId":          taskID,
			"workspaceId":     workspaceID,
			"canonicalPrompt": req.Prompt,
			"origin":          "api",
		})
		if _, err := svc.Handle(c.Request().Context(), tenant, actor, "CreateTask", payload); err != nil {
			code, category := app.ErrorCode(err)
			return c.JSON(statusFor(category), contracts.DomainError{Code: code, Message: err.Error()})
		}
		return c.JSON(http.StatusCreated, contracts.ManagedSession{
			Id: taskID, TaskId: &taskID, Status: contracts.Pending,
		})
	}
}

func getSession(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		tenant, _ := c.Get("tenant").(string)
		id := c.Param("id")
		view, found, err := svc.QueryTask(c.Request().Context(), tenant, id)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
		}
		if !found {
			return c.JSON(http.StatusNotFound, contracts.DomainError{Code: "not_found", Message: "session not found"})
		}
		taskID := view.ID
		return c.JSON(http.StatusOK, contracts.ManagedSession{
			Id: view.ID, TaskId: &taskID, Status: sessionStatus(view.Status),
		})
	}
}

func cancelSession(svc *app.Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		tenant, _ := c.Get("tenant").(string)
		actor, _ := c.Get("actor").(string)
		id := c.Param("id")
		payload, _ := json.Marshal(map[string]any{"taskId": id, "cancelledBy": actor})
		committed, err := svc.Handle(c.Request().Context(), tenant, actor, "CancelTask", payload)
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

// streamSessionEvents emits the task's events as Server-Sent Events. It backfills
// from the start then live-streams via hub pokes, filtering to this task's stream.
func streamSessionEvents(svc *app.Service, hub *realtime.Hub) echo.HandlerFunc {
	return func(c echo.Context) error {
		tenant, _ := c.Get("tenant").(string)
		streamID := "task:" + c.Param("id")

		w := c.Response()
		w.Header().Set(echo.HeaderContentType, "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		w.Flush()

		poke, release := hub.Subscribe()
		defer release()

		ctx := c.Request().Context()
		var cursor int64
		drain := func() error {
			for {
				evs, err := svc.EventsSince(ctx, tenant, cursor, 200)
				if err != nil || len(evs) == 0 {
					return err
				}
				for _, e := range evs {
					cursor = e.GlobalSeq
					if e.StreamID != streamID {
						continue
					}
					data, _ := json.Marshal(toCommittedEvent(e))
					if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
						return err
					}
					w.Flush()
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

// sessionStatus maps a read-model task status to the external session enum.
// "planned" is internal progress detail; externally it reads as running.
func sessionStatus(s string) contracts.ManagedSessionStatus {
	switch s {
	case "planned", "running":
		return contracts.Running
	case "awaiting_approval":
		return contracts.AwaitingApproval
	case "completed":
		return contracts.Completed
	case "failed":
		return contracts.Failed
	case "cancelled":
		return contracts.Cancelled
	default:
		return contracts.Pending
	}
}
