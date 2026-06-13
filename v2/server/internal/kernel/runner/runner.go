// Package runner is the LocalRunnerSession aggregate: a pure decision core
// (decide) plus event application (apply). It holds no I/O. This is the M3
// contract layer for the Local Agent Runner (spec 20.4): a runner registers via
// an outbound reverse channel, heartbeats, and goes stale on heartbeat loss.
// Invariants enforced here:
//   - heartbeat / mark-stale require a registered, non-stale runner;
//   - heartbeat pulses are monotonic (a stale/replayed pulse is rejected);
//   - a stale runner must re-register before it can heartbeat again.
//
// The real reverse gRPC/WS tunnel and liveness watchdog are a separate runtime
// project; this aggregate only models the session lifecycle as events.
package runner

import "errors"

// Domain errors (fail-fast, spec P6). One handling site maps these to codes.
var (
	ErrAlreadyExists = errors.New("runner: already registered")
	ErrNotFound      = errors.New("runner: not found")
	ErrStale         = errors.New("runner: stale, must re-register")
	ErrStalePulse    = errors.New("runner: non-monotonic heartbeat pulse")
)

// Runner is the aggregate state rebuilt by replaying events.
type Runner struct {
	exists    bool
	ID        string
	stale     bool
	lastPulse int
}

// Apply mutates state for one event (deterministic, no I/O).
func (r *Runner) Apply(e Event) {
	switch ev := e.(type) {
	case RunnerRegistered:
		r.exists = true
		r.ID = ev.RunnerID
		r.stale = false
		r.lastPulse = 0
	case RunnerHeartbeatPulsed:
		r.lastPulse = ev.Pulse
	case RunnerStale:
		r.stale = true
	}
}

// Load rebuilds a Runner from its event history.
func Load(history []Event) *Runner {
	r := &Runner{}
	for _, e := range history {
		r.Apply(e)
	}
	return r
}

// Decide validates a command against current state and invariants, returning
// the events to append (pure: no mutation, no I/O).
func (r *Runner) Decide(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case RegisterRunner:
		// Registration is allowed for a brand-new runner, or to recover a stale
		// one (re-register resets liveness). A live runner cannot re-register.
		if r.exists && !r.stale {
			return nil, ErrAlreadyExists
		}
		return []Event{RunnerRegistered{RunnerID: c.RunnerID, WorkspaceID: c.WorkspaceID, Capabilities: c.Capabilities}}, nil

	case RunnerHeartbeat:
		if err := r.live(); err != nil {
			return nil, err
		}
		if c.Pulse <= r.lastPulse {
			return nil, ErrStalePulse
		}
		return []Event{RunnerHeartbeatPulsed{RunnerID: r.ID, Pulse: c.Pulse}}, nil

	case MarkRunnerStale:
		if err := r.live(); err != nil {
			return nil, err
		}
		return []Event{RunnerStale{RunnerID: r.ID, Reason: c.Reason}}, nil

	default:
		return nil, errors.New("runner: unknown command")
	}
}

// live enforces that the runner exists and is not stale.
func (r *Runner) live() error {
	if !r.exists {
		return ErrNotFound
	}
	if r.stale {
		return ErrStale
	}
	return nil
}
