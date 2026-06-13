// Package workspace is the Workspace aggregate: a pure decision core (decide)
// plus event application (apply). It holds no I/O; the kernel handler loads
// events, calls Decide, and persists the result. Invariants (spec 7.2) are
// enforced here. Capability-first: permission changes are monotonic-versioned.
package workspace

import "errors"

// Domain errors (fail-fast, spec P6). One handling site maps these to codes.
var (
	ErrAlreadyExists = errors.New("workspace: already exists")
	ErrNotFound      = errors.New("workspace: not found")
)

// Workspace is the aggregate state rebuilt by replaying events.
type Workspace struct {
	exists             bool
	ID                 string
	Name               string
	PermissionsVersion int
}

// Apply mutates state for one event (deterministic, no I/O).
func (w *Workspace) Apply(e Event) {
	switch ev := e.(type) {
	case WorkspaceCreated:
		w.exists = true
		w.ID = ev.WorkspaceID
		w.Name = ev.Name
	case PermissionsChanged:
		w.PermissionsVersion = ev.Version
	}
}

// Load rebuilds a Workspace from its event history.
func Load(history []Event) *Workspace {
	w := &Workspace{}
	for _, e := range history {
		w.Apply(e)
	}
	return w
}

// Decide validates a command against current state and invariants, returning
// the events to append (pure: no mutation, no I/O).
func (w *Workspace) Decide(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case CreateWorkspace:
		if w.exists {
			return nil, ErrAlreadyExists
		}
		return []Event{WorkspaceCreated{WorkspaceID: c.WorkspaceID, Name: c.Name}}, nil

	case UpdatePermissions:
		if !w.exists {
			return nil, ErrNotFound
		}
		return []Event{PermissionsChanged{
			WorkspaceID: w.ID,
			Version:     w.PermissionsVersion + 1, // monotonic (capability-first)
			Permissions: c.Permissions,
		}}, nil

	default:
		return nil, errors.New("workspace: unknown command")
	}
}
