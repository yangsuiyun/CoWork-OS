package app

import (
	"encoding/json"
	"fmt"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/graph"
	"github.com/google/uuid"
)

func graphStreamID(id string) string { return "graph:" + id }

// decodeGraphCommand maps a wire command to an OrchestrationGraph domain command
// and its target stream (camelCase JSON boundary, mirrors decodeWorkspaceCommand).
func decodeGraphCommand(cmdType string, payload []byte) (graph.Command, string, error) {
	switch cmdType {
	case "SplitGraph":
		var p struct {
			GraphID string `json:"graphId"`
			TaskID  string `json:"taskId"`
			Nodes   []struct {
				NodeID         string   `json:"nodeId"`
				DispatchTarget string   `json:"dispatchTarget"`
				DependsOn      []string `json:"dependsOn"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.GraphID == "" {
			p.GraphID = uuid.NewString()
		}
		nodes := make([]graph.NodeSpec, 0, len(p.Nodes))
		for _, n := range p.Nodes {
			target := n.DispatchTarget
			if target == "" {
				target = "local"
			}
			nodes = append(nodes, graph.NodeSpec{NodeID: n.NodeID, DispatchTarget: target, DependsOn: n.DependsOn})
		}
		return graph.SplitGraph{GraphID: p.GraphID, TaskID: p.TaskID, Nodes: nodes}, graphStreamID(p.GraphID), nil

	case "DispatchNode":
		var p struct {
			GraphID      string `json:"graphId"`
			NodeID       string `json:"nodeId"`
			RemoteTaskID string `json:"remoteTaskId"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return graph.DispatchNode{GraphID: p.GraphID, NodeID: p.NodeID, RemoteTaskID: p.RemoteTaskID}, graphStreamID(p.GraphID), nil

	case "UpdateNode":
		var p struct {
			GraphID string `json:"graphId"`
			NodeID  string `json:"nodeId"`
			Status  string `json:"status"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return graph.UpdateNode{GraphID: p.GraphID, NodeID: p.NodeID, Status: p.Status, Outcome: p.Outcome}, graphStreamID(p.GraphID), nil

	case "MergeResult":
		var p struct {
			GraphID string `json:"graphId"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return graph.MergeResult{GraphID: p.GraphID, Outcome: p.Outcome}, graphStreamID(p.GraphID), nil

	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnknownCommand, cmdType)
	}
}

// graphReduce replays the Graph history, decides, and encodes events.
func graphReduce(cmd graph.Command, actor string, history []events.Committed) ([]events.ToAppend, error) {
	domainHist := make([]graph.Event, 0, len(history))
	for _, c := range history {
		de, err := graphCommittedToEvent(c)
		if err != nil {
			return nil, err
		}
		domainHist = append(domainHist, de)
	}
	newEvents, err := graph.Load(domainHist).Decide(cmd) // fail-fast
	if err != nil {
		return nil, err
	}
	out := make([]events.ToAppend, 0, len(newEvents))
	for _, e := range newEvents {
		payload, err := json.Marshal(graphEventPayload(e))
		if err != nil {
			return nil, err
		}
		out = append(out, events.ToAppend{Type: e.EventType(), SchemaVer: schemaVer, Payload: payload, Actor: actor})
	}
	return out, nil
}

func graphEventPayload(e graph.Event) any {
	switch ev := e.(type) {
	case graph.GraphSplit:
		nodes := make([]map[string]any, 0, len(ev.Nodes))
		for _, n := range ev.Nodes {
			nodes = append(nodes, map[string]any{
				"nodeId": n.NodeID, "dispatchTarget": n.DispatchTarget, "dependsOn": orEmpty(n.DependsOn),
			})
		}
		return map[string]any{"graphId": ev.GraphID, "taskId": ev.TaskID, "nodes": nodes}
	case graph.NodeDispatched:
		m := map[string]any{"graphId": ev.GraphID, "nodeId": ev.NodeID, "dispatchTarget": ev.DispatchTarget}
		if ev.RemoteTaskID != "" {
			m["remoteTaskId"] = ev.RemoteTaskID
		}
		return m
	case graph.NodeUpdated:
		m := map[string]any{"graphId": ev.GraphID, "nodeId": ev.NodeID, "status": ev.Status}
		if ev.Outcome != "" {
			m["outcome"] = ev.Outcome
		}
		return m
	case graph.ResultMerged:
		m := map[string]any{"graphId": ev.GraphID, "taskId": ev.TaskID}
		if ev.Outcome != "" {
			m["outcome"] = ev.Outcome
		}
		return m
	default:
		return map[string]any{}
	}
}

func graphCommittedToEvent(c events.Committed) (graph.Event, error) {
	switch c.Type {
	case "GraphSplit":
		var raw struct {
			GraphID string `json:"graphId"`
			TaskID  string `json:"taskId"`
			Nodes   []struct {
				NodeID         string   `json:"nodeId"`
				DispatchTarget string   `json:"dispatchTarget"`
				DependsOn      []string `json:"dependsOn"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		nodes := make([]graph.NodeSpec, 0, len(raw.Nodes))
		for _, n := range raw.Nodes {
			nodes = append(nodes, graph.NodeSpec{NodeID: n.NodeID, DispatchTarget: n.DispatchTarget, DependsOn: n.DependsOn})
		}
		return graph.GraphSplit{GraphID: raw.GraphID, TaskID: raw.TaskID, Nodes: nodes}, nil
	case "NodeDispatched":
		var raw struct {
			GraphID        string `json:"graphId"`
			NodeID         string `json:"nodeId"`
			DispatchTarget string `json:"dispatchTarget"`
			RemoteTaskID   string `json:"remoteTaskId"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return graph.NodeDispatched{GraphID: raw.GraphID, NodeID: raw.NodeID, DispatchTarget: raw.DispatchTarget, RemoteTaskID: raw.RemoteTaskID}, nil
	case "NodeUpdated":
		var raw struct {
			GraphID string `json:"graphId"`
			NodeID  string `json:"nodeId"`
			Status  string `json:"status"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return graph.NodeUpdated{GraphID: raw.GraphID, NodeID: raw.NodeID, Status: raw.Status, Outcome: raw.Outcome}, nil
	case "ResultMerged":
		var raw struct {
			GraphID string `json:"graphId"`
			TaskID  string `json:"taskId"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return graph.ResultMerged{GraphID: raw.GraphID, TaskID: raw.TaskID, Outcome: raw.Outcome}, nil
	default:
		return nil, fmt.Errorf("unknown graph event type: %s", c.Type)
	}
}
