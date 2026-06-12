// Package task is the Task aggregate: a pure decision core (decide) plus event
// application (apply). It holds no I/O; the kernel handler loads events, calls
// Decide, and persists the result. Invariants (spec 7.1) are enforced here.
package task

import "errors"

// Domain errors (fail-fast, spec P6). One handling site maps these to codes.
var (
	ErrAlreadyExists = errors.New("task: already exists")
	ErrNotFound      = errors.New("task: not found")
	ErrTerminal      = errors.New("task: terminal status, no further turns") // INV-2
	ErrNoParent      = errors.New("task: child task requires parentTaskId")  // INV-3
)

// Status is the task lifecycle state, derived from events (never written directly).
type Status string

const (
	StatusPending   Status = "pending"
	StatusPlanned   Status = "planned"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

func (s Status) terminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// Task is the aggregate state rebuilt by replaying events.
type Task struct {
	exists          bool
	ID              string
	WorkspaceID     string
	CanonicalPrompt string // INV-1: immutable once set
	ParentTaskID    string
	Status          Status
	Risk            string
}

// Apply mutates state for one event. Replaying all events reconstructs the
// aggregate (deterministic, no I/O).
func (t *Task) Apply(e Event) {
	switch ev := e.(type) {
	case TaskCreated:
		t.exists = true
		t.ID = ev.TaskID
		t.WorkspaceID = ev.WorkspaceID
		t.CanonicalPrompt = ev.CanonicalPrompt
		t.ParentTaskID = ev.ParentTaskID
		t.Risk = ev.Risk
		t.Status = StatusPending
	case TaskPlanned:
		t.Status = StatusPlanned
	case TurnStarted:
		t.Status = StatusRunning
	case TurnCompleted:
		t.Status = StatusPending
	case TaskCompleted:
		t.Status = StatusCompleted
	case TaskFailed:
		t.Status = StatusFailed
	case TaskCancelled:
		t.Status = StatusCancelled
	}
}

// Load rebuilds a Task from its event history.
func Load(history []Event) *Task {
	t := &Task{}
	for _, e := range history {
		t.Apply(e)
	}
	return t
}

// Decide validates a command against current state and invariants, returning
// the events to append (pure: no mutation, no I/O).
func (t *Task) Decide(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case CreateTask:
		if t.exists {
			return nil, ErrAlreadyExists
		}
		if c.IsChild && c.ParentTaskID == "" {
			return nil, ErrNoParent // INV-3
		}
		return []Event{TaskCreated{
			TaskID: c.TaskID, WorkspaceID: c.WorkspaceID, Origin: c.Origin,
			CanonicalPrompt: c.CanonicalPrompt, Risk: c.Risk, ParentTaskID: c.ParentTaskID,
		}}, nil

	case PlanTask:
		if err := t.requireActive(); err != nil {
			return nil, err
		}
		return []Event{TaskPlanned{TaskID: t.ID}}, nil

	case StartTurn:
		if err := t.requireActive(); err != nil {
			return nil, err
		}
		return []Event{TurnStarted{TaskID: t.ID, TurnID: c.TurnID}}, nil

	case CompleteTask:
		if err := t.requireActive(); err != nil {
			return nil, err
		}
		return []Event{TaskCompleted{TaskID: t.ID}}, nil

	case FailTask:
		if err := t.requireActive(); err != nil {
			return nil, err
		}
		return []Event{TaskFailed{TaskID: t.ID, ErrorCode: c.ErrorCode, Message: c.Message}}, nil

	case CancelTask:
		if err := t.requireActive(); err != nil {
			return nil, err
		}
		return []Event{TaskCancelled{TaskID: t.ID, CancelledBy: c.CancelledBy}}, nil

	default:
		return nil, errors.New("task: unknown command")
	}
}

// requireActive enforces existence and the non-terminal invariant (INV-2).
func (t *Task) requireActive() error {
	if !t.exists {
		return ErrNotFound
	}
	if t.Status.terminal() {
		return ErrTerminal
	}
	return nil
}
