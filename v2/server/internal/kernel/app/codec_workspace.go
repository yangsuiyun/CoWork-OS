package app

import (
	"encoding/json"
	"fmt"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/workspace"
	"github.com/google/uuid"
)

func workspaceStreamID(id string) string { return "workspace:" + id }

// decodeWorkspaceCommand maps a wire command to a Workspace domain command and
// its target stream (camelCase JSON boundary, mirrors decodeCommand for Task).
func decodeWorkspaceCommand(cmdType string, payload []byte) (workspace.Command, string, error) {
	switch cmdType {
	case "CreateWorkspace":
		var p struct {
			WorkspaceID string `json:"workspaceId"`
			Name        string `json:"name"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.WorkspaceID == "" {
			p.WorkspaceID = uuid.NewString()
		}
		return workspace.CreateWorkspace{WorkspaceID: p.WorkspaceID, Name: p.Name}, workspaceStreamID(p.WorkspaceID), nil
	case "UpdatePermissions":
		var p struct {
			WorkspaceID string `json:"workspaceId"`
			Permissions struct {
				Paths   []string `json:"paths"`
				Domains []string `json:"domains"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return workspace.UpdatePermissions{
			WorkspaceID: p.WorkspaceID,
			Permissions: workspace.Permissions{Paths: p.Permissions.Paths, Domains: p.Permissions.Domains},
		}, workspaceStreamID(p.WorkspaceID), nil
	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnknownCommand, cmdType)
	}
}

// workspaceReduce replays the Workspace history, decides, and encodes events.
func workspaceReduce(cmd workspace.Command, actor string, history []events.Committed) ([]events.ToAppend, error) {
	domainHist := make([]workspace.Event, 0, len(history))
	for _, c := range history {
		de, err := workspaceCommittedToEvent(c)
		if err != nil {
			return nil, err
		}
		domainHist = append(domainHist, de)
	}
	newEvents, err := workspace.Load(domainHist).Decide(cmd) // fail-fast
	if err != nil {
		return nil, err
	}
	out := make([]events.ToAppend, 0, len(newEvents))
	for _, e := range newEvents {
		payload, err := json.Marshal(workspaceEventPayload(e))
		if err != nil {
			return nil, err
		}
		out = append(out, events.ToAppend{Type: e.EventType(), SchemaVer: schemaVer, Payload: payload, Actor: actor})
	}
	return out, nil
}

func workspaceEventPayload(e workspace.Event) any {
	switch ev := e.(type) {
	case workspace.WorkspaceCreated:
		return map[string]any{"workspaceId": ev.WorkspaceID, "name": ev.Name}
	case workspace.PermissionsChanged:
		return map[string]any{
			"workspaceId": ev.WorkspaceID,
			"version":     ev.Version,
			"permissions": map[string]any{"paths": orEmpty(ev.Permissions.Paths), "domains": orEmpty(ev.Permissions.Domains)},
		}
	default:
		return map[string]any{}
	}
}

func workspaceCommittedToEvent(c events.Committed) (workspace.Event, error) {
	switch c.Type {
	case "WorkspaceCreated":
		var raw struct {
			WorkspaceID string `json:"workspaceId"`
			Name        string `json:"name"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return workspace.WorkspaceCreated{WorkspaceID: raw.WorkspaceID, Name: raw.Name}, nil
	case "PermissionsChanged":
		var raw struct {
			WorkspaceID string `json:"workspaceId"`
			Version     int    `json:"version"`
			Permissions struct {
				Paths   []string `json:"paths"`
				Domains []string `json:"domains"`
			} `json:"permissions"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return workspace.PermissionsChanged{
			WorkspaceID: raw.WorkspaceID, Version: raw.Version,
			Permissions: workspace.Permissions{Paths: raw.Permissions.Paths, Domains: raw.Permissions.Domains},
		}, nil
	default:
		return nil, fmt.Errorf("unknown workspace event type: %s", c.Type)
	}
}

// orEmpty normalizes a nil slice to an empty JSON array for stable payloads.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
