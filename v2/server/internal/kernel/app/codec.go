package app

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/task"
	"github.com/google/uuid"
)

const schemaVer = 1

// ErrUnknownCommand is returned for an unrecognized command type (client error).
var ErrUnknownCommand = errors.New("app: unknown command type")

func streamID(taskID string) string { return "task:" + taskID }

// decodeCommand maps a wire command (type + JSON payload) to a domain command
// and its target stream. This is the boundary between contract JSON (camelCase)
// and the domain types; PI-7 codegen will replace the hand-written DTOs.
func decodeCommand(cmdType string, payload []byte) (task.Command, string, error) {
	switch cmdType {
	case "CreateTask":
		var p struct {
			TaskID          string `json:"taskId"`
			WorkspaceID     string `json:"workspaceId"`
			Origin          string `json:"origin"`
			CanonicalPrompt string `json:"canonicalPrompt"`
			Risk            string `json:"risk"`
			ParentTaskID    string `json:"parentTaskId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.TaskID == "" {
			p.TaskID = uuid.NewString()
		}
		if p.Origin == "" {
			p.Origin = "api"
		}
		if p.Risk == "" {
			p.Risk = "low"
		}
		return task.CreateTask{
			TaskID: p.TaskID, WorkspaceID: p.WorkspaceID, Origin: p.Origin,
			CanonicalPrompt: p.CanonicalPrompt, Risk: p.Risk,
			IsChild: p.ParentTaskID != "", ParentTaskID: p.ParentTaskID,
		}, streamID(p.TaskID), nil

	case "PlanTask":
		id, err := taskIDOf(payload)
		return task.PlanTask{TaskID: id}, streamID(id), err
	case "StartTurn":
		var p struct {
			TaskID string `json:"taskId"`
			TurnID string `json:"turnId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.TurnID == "" {
			p.TurnID = uuid.NewString()
		}
		return task.StartTurn{TaskID: p.TaskID, TurnID: p.TurnID}, streamID(p.TaskID), nil
	case "CompleteTask":
		id, err := taskIDOf(payload)
		return task.CompleteTask{TaskID: id}, streamID(id), err
	case "FailTask":
		var p struct {
			TaskID    string `json:"taskId"`
			ErrorCode string `json:"errorCode"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return task.FailTask{TaskID: p.TaskID, ErrorCode: p.ErrorCode, Message: p.Message}, streamID(p.TaskID), nil
	case "CancelTask":
		var p struct {
			TaskID      string `json:"taskId"`
			CancelledBy string `json:"cancelledBy"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return task.CancelTask{TaskID: p.TaskID, CancelledBy: p.CancelledBy}, streamID(p.TaskID), nil
	case "RequestApproval":
		var p struct {
			TaskID     string         `json:"taskId"`
			ApprovalID string         `json:"approvalId"`
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
		return task.RequestApproval{TaskID: p.TaskID, ApprovalID: p.ApprovalID, Kind: p.Kind, Risk: p.Risk, Context: p.Context}, streamID(p.TaskID), nil
	case "ResolveApproval":
		var p struct {
			TaskID     string `json:"taskId"`
			ApprovalID string `json:"approvalId"`
			Decision   string `json:"decision"`
			ResolvedBy string `json:"resolvedBy"`
			Reason     string `json:"reason"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return task.ResolveApproval{TaskID: p.TaskID, ApprovalID: p.ApprovalID, Decision: p.Decision, ResolvedBy: p.ResolvedBy, Reason: p.Reason}, streamID(p.TaskID), nil
	case "AppendArtifact":
		var p struct {
			TaskID     string `json:"taskId"`
			ArtifactID string `json:"artifactId"`
			Path       string `json:"path"`
			SHA256     string `json:"sha256"`
			Mime       string `json:"mime"`
			Size       int64  `json:"size"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.ArtifactID == "" {
			p.ArtifactID = uuid.NewString()
		}
		return task.AppendArtifact{TaskID: p.TaskID, ArtifactID: p.ArtifactID, Path: p.Path, SHA256: p.SHA256, Mime: p.Mime, Size: p.Size}, streamID(p.TaskID), nil
	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnknownCommand, cmdType)
	}
}

func taskIDOf(payload []byte) (string, error) {
	var p struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	return p.TaskID, nil
}

// eventToAppend serializes a domain event into a store ToAppend (contract JSON).
func eventToAppend(e task.Event, actor string, correlationID *string) (events.ToAppend, error) {
	payload, err := json.Marshal(eventPayload(e))
	if err != nil {
		return events.ToAppend{}, err
	}
	return events.ToAppend{Type: e.EventType(), SchemaVer: schemaVer, Payload: payload, Actor: actor, CorrelationID: correlationID}, nil
}

func eventPayload(e task.Event) any {
	switch ev := e.(type) {
	case task.TaskCreated:
		return map[string]any{"taskId": ev.TaskID, "workspaceId": ev.WorkspaceID, "origin": ev.Origin, "canonicalPrompt": ev.CanonicalPrompt, "risk": ev.Risk, "parentTaskId": nullable(ev.ParentTaskID)}
	case task.TaskPlanned:
		return map[string]any{"taskId": ev.TaskID, "plan": []any{}}
	case task.TurnStarted:
		return map[string]any{"taskId": ev.TaskID, "turnId": ev.TurnID}
	case task.TurnCompleted:
		return map[string]any{"taskId": ev.TaskID, "turnId": ev.TurnID, "outcome": ev.Outcome}
	case task.TaskCompleted:
		return map[string]any{"taskId": ev.TaskID}
	case task.TaskFailed:
		return map[string]any{"taskId": ev.TaskID, "errorCode": ev.ErrorCode, "message": ev.Message}
	case task.TaskCancelled:
		return map[string]any{"taskId": ev.TaskID, "cancelledBy": ev.CancelledBy}
	case task.ApprovalRequested:
		m := map[string]any{"taskId": ev.TaskID, "approvalId": ev.ApprovalID, "kind": ev.Kind, "risk": ev.Risk}
		if ev.Context != nil {
			m["context"] = ev.Context
		}
		return m
	case task.ApprovalResolved:
		m := map[string]any{"taskId": ev.TaskID, "approvalId": ev.ApprovalID, "decision": ev.Decision, "resolvedBy": ev.ResolvedBy}
		if ev.Reason != "" {
			m["reason"] = ev.Reason
		}
		return m
	case task.ArtifactCreated:
		return map[string]any{"artifactId": ev.ArtifactID, "taskId": ev.TaskID, "path": ev.Path, "sha256": ev.SHA256, "mime": ev.Mime, "size": ev.Size}
	default:
		return map[string]any{}
	}
}

// committedToEvent rebuilds a domain event from a stored event for aggregate replay.
func committedToEvent(c events.Committed) (task.Event, error) {
	switch c.Type {
	case "TaskCreated":
		var raw struct {
			TaskID          string  `json:"taskId"`
			WorkspaceID     string  `json:"workspaceId"`
			Origin          string  `json:"origin"`
			CanonicalPrompt string  `json:"canonicalPrompt"`
			Risk            string  `json:"risk"`
			ParentTaskID    *string `json:"parentTaskId"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		parent := ""
		if raw.ParentTaskID != nil {
			parent = *raw.ParentTaskID
		}
		return task.TaskCreated{TaskID: raw.TaskID, WorkspaceID: raw.WorkspaceID, Origin: raw.Origin, CanonicalPrompt: raw.CanonicalPrompt, Risk: raw.Risk, ParentTaskID: parent}, nil
	case "TaskPlanned":
		return task.TaskPlanned{TaskID: jsonTaskID(c.Payload)}, nil
	case "TurnStarted":
		var raw struct {
			TaskID string `json:"taskId"`
			TurnID string `json:"turnId"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.TurnStarted{TaskID: raw.TaskID, TurnID: raw.TurnID}, nil
	case "TurnCompleted":
		var raw struct {
			TaskID  string `json:"taskId"`
			TurnID  string `json:"turnId"`
			Outcome string `json:"outcome"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.TurnCompleted{TaskID: raw.TaskID, TurnID: raw.TurnID, Outcome: raw.Outcome}, nil
	case "TaskCompleted":
		return task.TaskCompleted{TaskID: jsonTaskID(c.Payload)}, nil
	case "TaskFailed":
		var raw struct {
			TaskID    string `json:"taskId"`
			ErrorCode string `json:"errorCode"`
			Message   string `json:"message"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.TaskFailed{TaskID: raw.TaskID, ErrorCode: raw.ErrorCode, Message: raw.Message}, nil
	case "TaskCancelled":
		var raw struct {
			TaskID      string `json:"taskId"`
			CancelledBy string `json:"cancelledBy"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.TaskCancelled{TaskID: raw.TaskID, CancelledBy: raw.CancelledBy}, nil
	case "ApprovalRequested":
		var raw struct {
			TaskID     string         `json:"taskId"`
			ApprovalID string         `json:"approvalId"`
			Kind       string         `json:"kind"`
			Risk       string         `json:"risk"`
			Context    map[string]any `json:"context"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.ApprovalRequested{TaskID: raw.TaskID, ApprovalID: raw.ApprovalID, Kind: raw.Kind, Risk: raw.Risk, Context: raw.Context}, nil
	case "ApprovalResolved":
		var raw struct {
			TaskID     string `json:"taskId"`
			ApprovalID string `json:"approvalId"`
			Decision   string `json:"decision"`
			ResolvedBy string `json:"resolvedBy"`
			Reason     string `json:"reason"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.ApprovalResolved{TaskID: raw.TaskID, ApprovalID: raw.ApprovalID, Decision: raw.Decision, ResolvedBy: raw.ResolvedBy, Reason: raw.Reason}, nil
	case "ArtifactCreated":
		var raw struct {
			ArtifactID string `json:"artifactId"`
			TaskID     string `json:"taskId"`
			Path       string `json:"path"`
			SHA256     string `json:"sha256"`
			Mime       string `json:"mime"`
			Size       int64  `json:"size"`
		}
		_ = json.Unmarshal(c.Payload, &raw)
		return task.ArtifactCreated{ArtifactID: raw.ArtifactID, TaskID: raw.TaskID, Path: raw.Path, SHA256: raw.SHA256, Mime: raw.Mime, Size: raw.Size}, nil
	default:
		return nil, fmt.Errorf("unknown event type: %s", c.Type)
	}
}

func jsonTaskID(payload []byte) string {
	id, _ := taskIDOf(payload)
	return id
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
