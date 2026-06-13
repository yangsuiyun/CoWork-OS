package app

import (
	"encoding/json"
	"fmt"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/runner"
	"github.com/google/uuid"
)

func runnerStreamID(id string) string { return "runner:" + id }

// decodeRunnerCommand maps a wire command to a LocalRunnerSession domain command
// and its target stream (camelCase JSON boundary).
func decodeRunnerCommand(cmdType string, payload []byte) (runner.Command, string, error) {
	switch cmdType {
	case "RegisterRunner":
		var p struct {
			RunnerID     string   `json:"runnerId"`
			WorkspaceID  string   `json:"workspaceId"`
			Capabilities []string `json:"capabilities"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.RunnerID == "" {
			p.RunnerID = uuid.NewString()
		}
		return runner.RegisterRunner{RunnerID: p.RunnerID, WorkspaceID: p.WorkspaceID, Capabilities: p.Capabilities},
			runnerStreamID(p.RunnerID), nil
	case "RunnerHeartbeat":
		var p struct {
			RunnerID string `json:"runnerId"`
			Pulse    int    `json:"pulse"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return runner.RunnerHeartbeat{RunnerID: p.RunnerID, Pulse: p.Pulse}, runnerStreamID(p.RunnerID), nil
	case "MarkRunnerStale":
		var p struct {
			RunnerID string `json:"runnerId"`
			Reason   string `json:"reason"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return runner.MarkRunnerStale{RunnerID: p.RunnerID, Reason: p.Reason}, runnerStreamID(p.RunnerID), nil
	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnknownCommand, cmdType)
	}
}

// runnerReduce replays the Runner history, decides, and encodes events.
func runnerReduce(cmd runner.Command, actor string, history []events.Committed) ([]events.ToAppend, error) {
	domainHist := make([]runner.Event, 0, len(history))
	for _, c := range history {
		de, err := runnerCommittedToEvent(c)
		if err != nil {
			return nil, err
		}
		domainHist = append(domainHist, de)
	}
	newEvents, err := runner.Load(domainHist).Decide(cmd) // fail-fast
	if err != nil {
		return nil, err
	}
	out := make([]events.ToAppend, 0, len(newEvents))
	for _, e := range newEvents {
		payload, err := json.Marshal(runnerEventPayload(e))
		if err != nil {
			return nil, err
		}
		out = append(out, events.ToAppend{Type: e.EventType(), SchemaVer: schemaVer, Payload: payload, Actor: actor})
	}
	return out, nil
}

func runnerEventPayload(e runner.Event) any {
	switch ev := e.(type) {
	case runner.RunnerRegistered:
		return map[string]any{"runnerId": ev.RunnerID, "workspaceId": ev.WorkspaceID, "capabilities": orEmpty(ev.Capabilities)}
	case runner.RunnerHeartbeatPulsed:
		return map[string]any{"runnerId": ev.RunnerID, "pulse": ev.Pulse}
	case runner.RunnerStale:
		m := map[string]any{"runnerId": ev.RunnerID}
		if ev.Reason != "" {
			m["reason"] = ev.Reason
		}
		return m
	default:
		return map[string]any{}
	}
}

func runnerCommittedToEvent(c events.Committed) (runner.Event, error) {
	switch c.Type {
	case "RunnerRegistered":
		var raw struct {
			RunnerID     string   `json:"runnerId"`
			WorkspaceID  string   `json:"workspaceId"`
			Capabilities []string `json:"capabilities"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return runner.RunnerRegistered{RunnerID: raw.RunnerID, WorkspaceID: raw.WorkspaceID, Capabilities: raw.Capabilities}, nil
	case "RunnerHeartbeat":
		var raw struct {
			RunnerID string `json:"runnerId"`
			Pulse    int    `json:"pulse"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return runner.RunnerHeartbeatPulsed{RunnerID: raw.RunnerID, Pulse: raw.Pulse}, nil
	case "RunnerStale":
		var raw struct {
			RunnerID string `json:"runnerId"`
			Reason   string `json:"reason"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return runner.RunnerStale{RunnerID: raw.RunnerID, Reason: raw.Reason}, nil
	default:
		return nil, fmt.Errorf("unknown runner event type: %s", c.Type)
	}
}
