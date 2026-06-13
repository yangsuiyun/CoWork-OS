package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coworkos/cowork-os/v2/server/internal/cap"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
	"github.com/labstack/echo/v4"
)

// authorizeAction runs the full decision pipeline (spec 11.2): pre-hooks
// (deny/transform) -> mandatory capability check -> permission rule matrix ->
// post-hooks. Result: allow / ask (emit ApprovalRequested) / deny.
func authorizeAction(svc *app.Service, guard *cap.Guard) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req contracts.ActionRequest
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		tenant, _ := c.Get("tenant").(string)
		actor, _ := c.Get("actor").(string)
		ctx := c.Request().Context()

		// 1. Pre-hooks may deny or transform (never grant).
		hc := &cap.HookContext{Actor: actor, TaskID: req.TaskId, Resource: req.Resource, Risk: deref(req.Risk), Context: derefMap(req.Context)}
		outcome, err := guard.RunPre(ctx, hc)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
		}
		if outcome.Denied {
			return c.JSON(http.StatusForbidden, contracts.DomainError{Code: "hook_denied", Message: outcome.Reason})
		}

		// 2. Capability check (mandatory; never skippable). A token is "present"
		// only if it verifies AND its resource matches the requested resource.
		capabilityPresent := false
		if req.CapabilityToken != nil && *req.CapabilityToken != "" {
			token, err := guard.Verify(ctx, tenant, *req.CapabilityToken)
			if err != nil {
				switch {
				case errors.Is(err, cap.ErrRevoked), errors.Is(err, cap.ErrExpired), errors.Is(err, cap.ErrInvalidToken):
					return c.JSON(http.StatusForbidden, contracts.DomainError{Code: "capability_denied", Message: err.Error()})
				default:
					return c.JSON(http.StatusInternalServerError, contracts.DomainError{Code: "internal", Message: err.Error()})
				}
			}
			capabilityPresent = token.Resource == req.Resource
		}

		// 3. Permission rule matrix (includes uncoverable overrides).
		decision := cap.Decide(cap.Request{
			Resource:          req.Resource,
			Risk:              deref(req.Risk),
			InScope:           derefBool(req.InScope),
			OutsideScope:      derefBool(req.OutsideScope),
			DomainAllowListed: derefBool(req.DomainAllowListed),
			CapabilityPresent: capabilityPresent,
		})
		// 4. Post-hooks observe the decision (audit); failure is non-fatal here.
		_ = guard.RunPost(ctx, hc, decision.Decision)

		switch decision.Decision {
		case cap.Allow:
			return c.JSON(http.StatusOK, contracts.ActionResult{Decision: "allow", RuleId: decision.RuleID})

		case cap.Ask:
			// Record the approval requirement as a domain event on the task.
			payload, _ := json.Marshal(map[string]any{
				"taskId":  req.TaskId,
				"kind":    approvalKind(req.Resource),
				"risk":    orDefault(deref(req.Risk), "medium"),
				"context": derefMap(req.Context),
			})
			committed, err := svc.Handle(ctx, tenant, actor, "RequestApproval", payload)
			if err != nil {
				code, category := app.ErrorCode(err)
				return c.JSON(statusFor(category), contracts.DomainError{Code: code, Message: err.Error()})
			}
			events := make([]contracts.CommittedEvent, 0, len(committed))
			for _, e := range committed {
				events = append(events, toCommittedEvent(e))
			}
			return c.JSON(http.StatusOK, contracts.ActionResult{Decision: "ask", RuleId: decision.RuleID, Events: &events})

		default: // deny
			return c.JSON(http.StatusForbidden, contracts.DomainError{
				Code: "permission_denied", Message: "action denied by rule " + decision.RuleID,
				Details: &map[string]any{"ruleId": decision.RuleID},
			})
		}
	}
}

// approvalKind maps a resource class to the approval kind enum.
func approvalKind(resource string) string {
	switch resource {
	case "fs.write":
		return "fs_write"
	case "net", "shell", "data_export":
		return resource
	default:
		return "tool"
	}
}

func deref(r *contracts.ActionRequestRisk) string {
	if r == nil {
		return ""
	}
	return string(*r)
}

func derefBool(b *bool) bool { return b != nil && *b }

func derefMap(m *map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return *m
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
