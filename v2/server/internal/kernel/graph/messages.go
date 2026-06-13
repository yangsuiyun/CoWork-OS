package graph

// Command is a graph command. CommandType matches the contract command name.
type Command interface{ CommandType() string }

// Event is a graph domain event. EventType matches the catalog event type.
type Event interface{ EventType() string }

// NodeSpec declares one DAG node at split time (spec 12.1). dispatchTarget is
// the only thing that differs between a local sub-agent and a remote Agent.
type NodeSpec struct {
	NodeID         string
	DispatchTarget string // local | remote
	DependsOn      []string
}

// --- Commands ---

type SplitGraph struct {
	GraphID string
	TaskID  string
	Nodes   []NodeSpec
}

type DispatchNode struct {
	GraphID      string
	NodeID       string
	RemoteTaskID string // set when the node's dispatchTarget is remote
}

type UpdateNode struct {
	GraphID string
	NodeID  string
	Status  string // done | failed
	Outcome string
}

type MergeResult struct {
	GraphID string
	Outcome string
}

func (SplitGraph) CommandType() string   { return "SplitGraph" }
func (DispatchNode) CommandType() string { return "DispatchNode" }
func (UpdateNode) CommandType() string   { return "UpdateNode" }
func (MergeResult) CommandType() string  { return "MergeResult" }

// --- Events --- (payloads mirror contracts/events/*.schema.json)

type GraphSplit struct {
	GraphID string
	TaskID  string
	Nodes   []NodeSpec
}

type NodeDispatched struct {
	GraphID        string
	NodeID         string
	DispatchTarget string
	RemoteTaskID   string
}

type NodeUpdated struct {
	GraphID string
	NodeID  string
	Status  string
	Outcome string
}

type ResultMerged struct {
	GraphID string
	TaskID  string
	Outcome string
}

func (GraphSplit) EventType() string     { return "GraphSplit" }
func (NodeDispatched) EventType() string { return "NodeDispatched" }
func (NodeUpdated) EventType() string    { return "NodeUpdated" }
func (ResultMerged) EventType() string   { return "ResultMerged" }
