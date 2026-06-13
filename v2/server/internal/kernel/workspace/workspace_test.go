package workspace

import (
	"errors"
	"testing"
)

func TestCreateThenUpdatePermissions(t *testing.T) {
	w := Load(nil)

	evs, err := w.Decide(CreateWorkspace{WorkspaceID: "ws1", Name: "Default"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if _, ok := evs[0].(WorkspaceCreated); !ok {
		t.Fatalf("expected WorkspaceCreated, got %T", evs[0])
	}

	// Replay so the aggregate reflects creation.
	w.Apply(evs[0])

	evs, err = w.Decide(UpdatePermissions{WorkspaceID: "ws1", Permissions: Permissions{Paths: []string{"/repo"}}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	pc, ok := evs[0].(PermissionsChanged)
	if !ok {
		t.Fatalf("expected PermissionsChanged, got %T", evs[0])
	}
	if pc.Version != 1 {
		t.Fatalf("first permissions change should be version 1, got %d", pc.Version)
	}

	// Versions are monotonic across replays.
	w.Apply(pc)
	evs, _ = w.Decide(UpdatePermissions{WorkspaceID: "ws1", Permissions: Permissions{Domains: []string{"example.com"}}})
	if evs[0].(PermissionsChanged).Version != 2 {
		t.Fatalf("second change should be version 2, got %d", evs[0].(PermissionsChanged).Version)
	}
}

func TestInvariants(t *testing.T) {
	// Duplicate create rejected.
	w := Load([]Event{WorkspaceCreated{WorkspaceID: "ws1", Name: "x"}})
	if _, err := w.Decide(CreateWorkspace{WorkspaceID: "ws1"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate create want ErrAlreadyExists, got %v", err)
	}

	// Update before create rejected.
	empty := Load(nil)
	if _, err := empty.Decide(UpdatePermissions{WorkspaceID: "ws1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing want ErrNotFound, got %v", err)
	}
}
