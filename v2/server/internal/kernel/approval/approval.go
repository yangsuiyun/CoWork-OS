// Package approval is the ApprovalRequest aggregate: a pure decision core
// (decide) plus event application (apply). It holds no I/O. Invariants
// (spec 7.2) are enforced here: a resolved approval is terminal, so a
// hard-deny (reject) can never be overridden by a later approve.
package approval

import "errors"

// Domain errors (fail-fast, spec P6). One handling site maps these to codes.
var (
	ErrAlreadyExists = errors.New("approval: already exists")
	ErrNotFound      = errors.New("approval: not found")
	ErrResolved      = errors.New("approval: already resolved, cannot override") // hard-deny non-overridable
)

// Approval is the aggregate state rebuilt by replaying events.
type Approval struct {
	exists   bool
	ID       string
	TaskID   string
	resolved bool
}

// Apply mutates state for one event (deterministic, no I/O).
func (a *Approval) Apply(e Event) {
	switch ev := e.(type) {
	case ApprovalRequested:
		a.exists = true
		a.ID = ev.ApprovalID
		a.TaskID = ev.TaskID
	case ApprovalResolved:
		a.resolved = true
	}
}

// Load rebuilds an Approval from its event history.
func Load(history []Event) *Approval {
	a := &Approval{}
	for _, e := range history {
		a.Apply(e)
	}
	return a
}

// Decide validates a command against current state and invariants, returning
// the events to append (pure: no mutation, no I/O).
func (a *Approval) Decide(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case RequestApproval:
		if a.exists {
			return nil, ErrAlreadyExists
		}
		return []Event{ApprovalRequested{
			ApprovalID: c.ApprovalID, TaskID: c.TaskID, Kind: c.Kind, Risk: c.Risk, Context: c.Context,
		}}, nil

	case ApproveApproval:
		return a.resolve("approve", c.ResolvedBy, c.Reason)
	case RejectApproval:
		return a.resolve("reject", c.ResolvedBy, c.Reason)

	default:
		return nil, errors.New("approval: unknown command")
	}
}

// resolve enforces existence and the non-overridable invariant, then emits the
// resolution event.
func (a *Approval) resolve(decision, by, reason string) ([]Event, error) {
	if !a.exists {
		return nil, ErrNotFound
	}
	if a.resolved {
		return nil, ErrResolved // hard-deny / prior decision is final
	}
	return []Event{ApprovalResolved{
		ApprovalID: a.ID, TaskID: a.TaskID, Decision: decision, ResolvedBy: by, Reason: reason,
	}}, nil
}
