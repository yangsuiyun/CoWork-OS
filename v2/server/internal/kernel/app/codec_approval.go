package app

import (
	"encoding/json"
	"fmt"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/approval"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/google/uuid"
)

func approvalStreamID(id string) string { return "approval:" + id }

// decodeApprovalCommand maps a wire command to an Approval domain command and
// its target stream (camelCase JSON boundary).
func decodeApprovalCommand(cmdType string, payload []byte) (approval.Command, string, error) {
	switch cmdType {
	case "RequestApproval":
		var p struct {
			ApprovalID string         `json:"approvalId"`
			TaskID     string         `json:"taskId"`
			Kind       string         `json:"kind"`
			Risk       string         `json:"risk"`
			Context    map[string]any `json:"context"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.ApprovalID == "" {
			p.ApprovalID = uuid.NewString()
		}
		return approval.RequestApproval{ApprovalID: p.ApprovalID, TaskID: p.TaskID, Kind: p.Kind, Risk: p.Risk, Context: p.Context},
			approvalStreamID(p.ApprovalID), nil
	case "ApproveApproval", "RejectApproval":
		var p struct {
			ApprovalID string `json:"approvalId"`
			ResolvedBy string `json:"resolvedBy"`
			Reason     string `json:"reason"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		stream := approvalStreamID(p.ApprovalID)
		if cmdType == "ApproveApproval" {
			return approval.ApproveApproval{ApprovalID: p.ApprovalID, ResolvedBy: p.ResolvedBy, Reason: p.Reason}, stream, nil
		}
		return approval.RejectApproval{ApprovalID: p.ApprovalID, ResolvedBy: p.ResolvedBy, Reason: p.Reason}, stream, nil
	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnknownCommand, cmdType)
	}
}

// approvalReduce replays the Approval history, decides, and encodes events.
func approvalReduce(cmd approval.Command, actor string, history []events.Committed) ([]events.ToAppend, error) {
	domainHist := make([]approval.Event, 0, len(history))
	for _, c := range history {
		de, err := approvalCommittedToEvent(c)
		if err != nil {
			return nil, err
		}
		domainHist = append(domainHist, de)
	}
	newEvents, err := approval.Load(domainHist).Decide(cmd) // fail-fast
	if err != nil {
		return nil, err
	}
	out := make([]events.ToAppend, 0, len(newEvents))
	for _, e := range newEvents {
		payload, err := json.Marshal(approvalEventPayload(e))
		if err != nil {
			return nil, err
		}
		out = append(out, events.ToAppend{Type: e.EventType(), SchemaVer: schemaVer, Payload: payload, Actor: actor})
	}
	return out, nil
}

func approvalEventPayload(e approval.Event) any {
	switch ev := e.(type) {
	case approval.ApprovalRequested:
		m := map[string]any{"taskId": ev.TaskID, "approvalId": ev.ApprovalID, "kind": ev.Kind, "risk": ev.Risk}
		if ev.Context != nil {
			m["context"] = ev.Context
		}
		return m
	case approval.ApprovalResolved:
		m := map[string]any{"taskId": ev.TaskID, "approvalId": ev.ApprovalID, "decision": ev.Decision, "resolvedBy": ev.ResolvedBy}
		if ev.Reason != "" {
			m["reason"] = ev.Reason
		}
		return m
	default:
		return map[string]any{}
	}
}

func approvalCommittedToEvent(c events.Committed) (approval.Event, error) {
	switch c.Type {
	case "ApprovalRequested":
		var raw struct {
			TaskID     string         `json:"taskId"`
			ApprovalID string         `json:"approvalId"`
			Kind       string         `json:"kind"`
			Risk       string         `json:"risk"`
			Context    map[string]any `json:"context"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return approval.ApprovalRequested{ApprovalID: raw.ApprovalID, TaskID: raw.TaskID, Kind: raw.Kind, Risk: raw.Risk, Context: raw.Context}, nil
	case "ApprovalResolved":
		var raw struct {
			TaskID     string `json:"taskId"`
			ApprovalID string `json:"approvalId"`
			Decision   string `json:"decision"`
			ResolvedBy string `json:"resolvedBy"`
			Reason     string `json:"reason"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return approval.ApprovalResolved{ApprovalID: raw.ApprovalID, TaskID: raw.TaskID, Decision: raw.Decision, ResolvedBy: raw.ResolvedBy, Reason: raw.Reason}, nil
	default:
		return nil, fmt.Errorf("unknown approval event type: %s", c.Type)
	}
}
