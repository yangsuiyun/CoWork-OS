// Package graph is the OrchestrationGraph aggregate: a pure decision core
// (decide) plus event application (apply). It holds no I/O. Invariants
// (spec 12.1) are enforced here:
//   - a node may only be dispatched/updated if it belongs to the graph;
//   - a node's outcome is terminal (done/failed cannot be overwritten);
//   - once the graph result is merged, it is closed: no further dispatch/merge.
//
// Local sub-agents and remote Agents share one node abstraction; the only
// difference is dispatchTarget, kept on the node spec.
package graph

import "errors"

// Domain errors (fail-fast, spec P6). One handling site maps these to codes.
var (
	ErrAlreadyExists = errors.New("graph: already exists")
	ErrNotFound      = errors.New("graph: not found")
	ErrNoNode        = errors.New("graph: node not found")
	ErrNodeTerminal  = errors.New("graph: node already terminal")
	ErrMerged        = errors.New("graph: already merged, closed")
	ErrEmpty         = errors.New("graph: must declare at least one node")
)

type nodeState struct {
	dispatchTarget string
	status         string // pending | dispatched | done | failed
}

// Graph is the aggregate state rebuilt by replaying events.
type Graph struct {
	exists bool
	ID     string
	TaskID string
	nodes  map[string]*nodeState
	merged bool
}

// Apply mutates state for one event (deterministic, no I/O).
func (g *Graph) Apply(e Event) {
	switch ev := e.(type) {
	case GraphSplit:
		g.exists = true
		g.ID = ev.GraphID
		g.TaskID = ev.TaskID
		g.nodes = make(map[string]*nodeState, len(ev.Nodes))
		for _, n := range ev.Nodes {
			g.nodes[n.NodeID] = &nodeState{dispatchTarget: n.DispatchTarget, status: "pending"}
		}
	case NodeDispatched:
		if n := g.nodes[ev.NodeID]; n != nil {
			n.status = "dispatched"
		}
	case NodeUpdated:
		if n := g.nodes[ev.NodeID]; n != nil {
			n.status = ev.Status
		}
	case ResultMerged:
		g.merged = true
	}
}

// Load rebuilds a Graph from its event history.
func Load(history []Event) *Graph {
	g := &Graph{}
	for _, e := range history {
		g.Apply(e)
	}
	return g
}

// Decide validates a command against current state and invariants, returning
// the events to append (pure: no mutation, no I/O).
func (g *Graph) Decide(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case SplitGraph:
		if g.exists {
			return nil, ErrAlreadyExists
		}
		if len(c.Nodes) == 0 {
			return nil, ErrEmpty
		}
		return []Event{GraphSplit{GraphID: c.GraphID, TaskID: c.TaskID, Nodes: c.Nodes}}, nil

	case DispatchNode:
		n, err := g.openNode(c.NodeID)
		if err != nil {
			return nil, err
		}
		return []Event{NodeDispatched{
			GraphID: g.ID, NodeID: c.NodeID, DispatchTarget: n.dispatchTarget, RemoteTaskID: c.RemoteTaskID,
		}}, nil

	case UpdateNode:
		if _, err := g.openNode(c.NodeID); err != nil {
			return nil, err
		}
		return []Event{NodeUpdated{GraphID: g.ID, NodeID: c.NodeID, Status: c.Status, Outcome: c.Outcome}}, nil

	case MergeResult:
		if !g.exists {
			return nil, ErrNotFound
		}
		if g.merged {
			return nil, ErrMerged
		}
		return []Event{ResultMerged{GraphID: g.ID, TaskID: g.TaskID, Outcome: c.Outcome}}, nil

	default:
		return nil, errors.New("graph: unknown command")
	}
}

// openNode resolves a node that can still transition: the graph must exist and
// not be merged, the node must exist and not be terminal (done/failed).
func (g *Graph) openNode(nodeID string) (*nodeState, error) {
	if !g.exists {
		return nil, ErrNotFound
	}
	if g.merged {
		return nil, ErrMerged
	}
	n := g.nodes[nodeID]
	if n == nil {
		return nil, ErrNoNode
	}
	if n.status == "done" || n.status == "failed" {
		return nil, ErrNodeTerminal
	}
	return n, nil
}
