package runner

import (
	"errors"
	"testing"
)

func TestRegisterHeartbeatStale(t *testing.T) {
	r := Load(nil)

	evs, err := r.Decide(RegisterRunner{RunnerID: "r1", WorkspaceID: "ws1", Capabilities: []string{"shell"}})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := evs[0].(RunnerRegistered); !ok {
		t.Fatalf("expected RunnerRegistered, got %T", evs[0])
	}
	r.Apply(evs[0])

	// Monotonic heartbeats advance.
	for _, pulse := range []int{1, 2, 5} {
		evs, err = r.Decide(RunnerHeartbeat{RunnerID: "r1", Pulse: pulse})
		if err != nil {
			t.Fatalf("heartbeat %d: %v", pulse, err)
		}
		r.Apply(evs[0])
	}

	evs, err = r.Decide(MarkRunnerStale{RunnerID: "r1", Reason: "heartbeat_lost"})
	if err != nil {
		t.Fatalf("stale: %v", err)
	}
	if _, ok := evs[0].(RunnerStale); !ok {
		t.Fatalf("expected RunnerStale, got %T", evs[0])
	}
}

func TestInvariants(t *testing.T) {
	// Live runner cannot re-register.
	live := Load([]Event{RunnerRegistered{RunnerID: "r1", WorkspaceID: "ws1"}})
	if _, err := live.Decide(RegisterRunner{RunnerID: "r1", WorkspaceID: "ws1"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("re-register live want ErrAlreadyExists, got %v", err)
	}

	// Heartbeat before register rejected.
	empty := Load(nil)
	if _, err := empty.Decide(RunnerHeartbeat{RunnerID: "r1", Pulse: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("heartbeat missing want ErrNotFound, got %v", err)
	}

	// Non-monotonic pulse rejected.
	beating := Load([]Event{
		RunnerRegistered{RunnerID: "r1", WorkspaceID: "ws1"},
		RunnerHeartbeatPulsed{RunnerID: "r1", Pulse: 5},
	})
	if _, err := beating.Decide(RunnerHeartbeat{RunnerID: "r1", Pulse: 5}); !errors.Is(err, ErrStalePulse) {
		t.Fatalf("stale pulse want ErrStalePulse, got %v", err)
	}

	// Stale runner cannot heartbeat; must re-register first.
	stale := Load([]Event{
		RunnerRegistered{RunnerID: "r1", WorkspaceID: "ws1"},
		RunnerStale{RunnerID: "r1", Reason: "heartbeat_lost"},
	})
	if _, err := stale.Decide(RunnerHeartbeat{RunnerID: "r1", Pulse: 10}); !errors.Is(err, ErrStale) {
		t.Fatalf("heartbeat while stale want ErrStale, got %v", err)
	}
	// Re-registration recovers a stale runner.
	if _, err := stale.Decide(RegisterRunner{RunnerID: "r1", WorkspaceID: "ws1"}); err != nil {
		t.Fatalf("re-register stale should recover, got %v", err)
	}
}
