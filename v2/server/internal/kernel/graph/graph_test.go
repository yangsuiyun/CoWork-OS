package graph

import (
	"errors"
	"testing"
)

func TestSplitDispatchUpdateMerge(t *testing.T) {
	g := Load(nil)

	evs, err := g.Decide(SplitGraph{GraphID: "g1", TaskID: "t1", Nodes: []NodeSpec{
		{NodeID: "n1", DispatchTarget: "local"},
		{NodeID: "n2", DispatchTarget: "remote"},
	}})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if _, ok := evs[0].(GraphSplit); !ok {
		t.Fatalf("expected GraphSplit, got %T", evs[0])
	}
	g.Apply(evs[0])

	// Remote node dispatch carries the converging remoteTaskId and target.
	evs, err = g.Decide(DispatchNode{GraphID: "g1", NodeID: "n2", RemoteTaskID: "rt9"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	nd, ok := evs[0].(NodeDispatched)
	if !ok || nd.DispatchTarget != "remote" || nd.RemoteTaskID != "rt9" {
		t.Fatalf("unexpected dispatch event: %+v", evs[0])
	}
	g.Apply(evs[0])

	evs, err = g.Decide(UpdateNode{GraphID: "g1", NodeID: "n2", Status: "done", Outcome: "ok"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	g.Apply(evs[0])

	evs, err = g.Decide(MergeResult{GraphID: "g1", Outcome: "merged"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	rm, ok := evs[0].(ResultMerged)
	if !ok || rm.TaskID != "t1" {
		t.Fatalf("unexpected merge event: %+v", evs[0])
	}
}

func TestInvariants(t *testing.T) {
	// Split with no nodes rejected.
	empty := Load(nil)
	if _, err := empty.Decide(SplitGraph{GraphID: "g1", TaskID: "t1"}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty split want ErrEmpty, got %v", err)
	}
	for name, nodes := range map[string][]NodeSpec{
		"empty node id":    {{NodeID: "", DispatchTarget: "local"}},
		"invalid target":   {{NodeID: "n1", DispatchTarget: "sideways"}},
		"duplicate node":   {{NodeID: "n1", DispatchTarget: "local"}, {NodeID: "n1", DispatchTarget: "remote"}},
		"missing dep":      {{NodeID: "n1", DispatchTarget: "local", DependsOn: []string{"n2"}}},
		"self dep":         {{NodeID: "n1", DispatchTarget: "local", DependsOn: []string{"n1"}}},
		"dependency cycle": {{NodeID: "n1", DispatchTarget: "local", DependsOn: []string{"n2"}}, {NodeID: "n2", DispatchTarget: "remote", DependsOn: []string{"n1"}}},
	} {
		if _, err := empty.Decide(SplitGraph{GraphID: "g1", TaskID: "t1", Nodes: nodes}); !errors.Is(err, ErrInvalidDAG) {
			t.Fatalf("%s want ErrInvalidDAG, got %v", name, err)
		}
	}

	// Duplicate split rejected.
	g := Load([]Event{GraphSplit{GraphID: "g1", TaskID: "t1", Nodes: []NodeSpec{{NodeID: "n1", DispatchTarget: "local"}}}})
	if _, err := g.Decide(SplitGraph{GraphID: "g1", TaskID: "t1", Nodes: []NodeSpec{{NodeID: "n1"}}}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate split want ErrAlreadyExists, got %v", err)
	}
	if _, err := g.Decide(UpdateNode{GraphID: "g1", NodeID: "n1", Status: "running"}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("invalid status want ErrInvalidStatus, got %v", err)
	}

	// Dispatch on unknown graph rejected.
	if _, err := empty.Decide(DispatchNode{GraphID: "g1", NodeID: "n1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dispatch missing graph want ErrNotFound, got %v", err)
	}

	// Dispatch on unknown node rejected.
	if _, err := g.Decide(DispatchNode{GraphID: "g1", NodeID: "nX"}); !errors.Is(err, ErrNoNode) {
		t.Fatalf("dispatch missing node want ErrNoNode, got %v", err)
	}

	// Terminal node cannot transition again.
	done := Load([]Event{
		GraphSplit{GraphID: "g1", TaskID: "t1", Nodes: []NodeSpec{{NodeID: "n1", DispatchTarget: "local"}}},
		NodeUpdated{GraphID: "g1", NodeID: "n1", Status: "done"},
	})
	if _, err := done.Decide(UpdateNode{GraphID: "g1", NodeID: "n1", Status: "failed"}); !errors.Is(err, ErrNodeTerminal) {
		t.Fatalf("terminal node want ErrNodeTerminal, got %v", err)
	}

	// Merged graph is closed: no further dispatch or merge.
	merged := Load([]Event{
		GraphSplit{GraphID: "g1", TaskID: "t1", Nodes: []NodeSpec{{NodeID: "n1", DispatchTarget: "local"}}},
		ResultMerged{GraphID: "g1", TaskID: "t1"},
	})
	if _, err := merged.Decide(MergeResult{GraphID: "g1"}); !errors.Is(err, ErrMerged) {
		t.Fatalf("re-merge want ErrMerged, got %v", err)
	}
	if _, err := merged.Decide(DispatchNode{GraphID: "g1", NodeID: "n1"}); !errors.Is(err, ErrMerged) {
		t.Fatalf("dispatch after merge want ErrMerged, got %v", err)
	}
}
