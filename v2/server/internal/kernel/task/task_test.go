package task

import (
	"errors"
	"testing"
)

func TestCreateThenRejectDuplicate(t *testing.T) {
	agg := Load(nil)
	evs, err := agg.Decide(CreateTask{TaskID: "1", WorkspaceID: "w", Origin: "manual", CanonicalPrompt: "do x", Risk: "low"})
	if err != nil || len(evs) != 1 {
		t.Fatalf("create: evs=%v err=%v", evs, err)
	}
	if _, ok := evs[0].(TaskCreated); !ok {
		t.Fatalf("expected TaskCreated, got %T", evs[0])
	}

	agg.Apply(evs[0])
	if _, err := agg.Decide(CreateTask{TaskID: "1"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestCommandOnMissingTask(t *testing.T) {
	agg := Load(nil)
	if _, err := agg.Decide(StartTurn{TaskID: "1", TurnID: "t1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// INV-2: terminal task rejects further turns.
func TestTerminalRejectsTurn(t *testing.T) {
	agg := Load([]Event{
		TaskCreated{TaskID: "1", WorkspaceID: "w", CanonicalPrompt: "p", Risk: "low"},
		TaskCompleted{TaskID: "1"},
	})
	if agg.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", agg.Status)
	}
	if _, err := agg.Decide(StartTurn{TaskID: "1", TurnID: "t1"}); !errors.Is(err, ErrTerminal) {
		t.Fatalf("expected ErrTerminal, got %v", err)
	}
}

// INV-3: child task requires a parent.
func TestChildRequiresParent(t *testing.T) {
	agg := Load(nil)
	if _, err := agg.Decide(CreateTask{TaskID: "c", IsChild: true}); !errors.Is(err, ErrNoParent) {
		t.Fatalf("expected ErrNoParent, got %v", err)
	}
	if _, err := agg.Decide(CreateTask{TaskID: "c", IsChild: true, ParentTaskID: "p", WorkspaceID: "w", CanonicalPrompt: "x", Risk: "low"}); err != nil {
		t.Fatalf("child with parent should pass, got %v", err)
	}
}

// INV-1: canonicalPrompt is set once and never mutated by later events.
func TestCanonicalPromptImmutable(t *testing.T) {
	agg := Load([]Event{
		TaskCreated{TaskID: "1", CanonicalPrompt: "original", Risk: "low"},
		TaskPlanned{TaskID: "1"},
		TurnStarted{TaskID: "1", TurnID: "t1"},
	})
	if agg.CanonicalPrompt != "original" {
		t.Fatalf("canonicalPrompt mutated: %q", agg.CanonicalPrompt)
	}
}

func TestAppendArtifactKeepsStatus(t *testing.T) {
	agg := Load([]Event{
		TaskCreated{TaskID: "1", WorkspaceID: "w", CanonicalPrompt: "p", Risk: "low"},
		TaskPlanned{TaskID: "1"},
	})
	evs, err := agg.Decide(AppendArtifact{TaskID: "1", ArtifactID: "art1", Path: "/out.txt", SHA256: "abc", Mime: "text/plain", Size: 12})
	if err != nil {
		t.Fatalf("append artifact: %v", err)
	}
	if _, ok := evs[0].(ArtifactCreated); !ok {
		t.Fatalf("expected ArtifactCreated, got %T", evs[0])
	}
	agg.Apply(evs[0])
	if agg.Status != StatusPlanned {
		t.Fatalf("artifact must not change status, got %s", agg.Status)
	}
}

func TestLifecycleStatuses(t *testing.T) {
	agg := Load(nil)
	steps := []struct {
		cmd  Command
		want Status
	}{
		{CreateTask{TaskID: "1", WorkspaceID: "w", CanonicalPrompt: "p", Risk: "low"}, StatusPending},
		{PlanTask{TaskID: "1"}, StatusPlanned},
		{StartTurn{TaskID: "1", TurnID: "t1"}, StatusRunning},
		{CompleteTask{TaskID: "1"}, StatusCompleted},
	}
	for i, s := range steps {
		evs, err := agg.Decide(s.cmd)
		if err != nil {
			t.Fatalf("step %d %s: %v", i, s.cmd.CommandType(), err)
		}
		for _, e := range evs {
			agg.Apply(e)
		}
		if agg.Status != s.want {
			t.Fatalf("step %d: want %s got %s", i, s.want, agg.Status)
		}
	}
}
