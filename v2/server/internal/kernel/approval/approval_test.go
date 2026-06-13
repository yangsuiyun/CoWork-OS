package approval

import (
	"errors"
	"testing"
)

func TestRequestThenResolve(t *testing.T) {
	a := Load(nil)

	evs, err := a.Decide(RequestApproval{ApprovalID: "ap1", TaskID: "t1", Kind: "shell", Risk: "high"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, ok := evs[0].(ApprovalRequested); !ok {
		t.Fatalf("expected ApprovalRequested, got %T", evs[0])
	}
	a.Apply(evs[0])

	evs, err = a.Decide(ApproveApproval{ApprovalID: "ap1", ResolvedBy: "human"})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	res, ok := evs[0].(ApprovalResolved)
	if !ok || res.Decision != "approve" || res.TaskID != "t1" {
		t.Fatalf("unexpected resolve event: %+v", evs[0])
	}
}

func TestInvariants(t *testing.T) {
	// Duplicate request rejected.
	a := Load([]Event{ApprovalRequested{ApprovalID: "ap1", TaskID: "t1", Kind: "tool", Risk: "low"}})
	if _, err := a.Decide(RequestApproval{ApprovalID: "ap1", TaskID: "t1"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate request want ErrAlreadyExists, got %v", err)
	}

	// Resolve before request rejected.
	empty := Load(nil)
	if _, err := empty.Decide(ApproveApproval{ApprovalID: "ap1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve missing want ErrNotFound, got %v", err)
	}

	// hard-deny non-overridable: once rejected, approve is refused.
	rejected := Load([]Event{
		ApprovalRequested{ApprovalID: "ap1", TaskID: "t1", Kind: "shell", Risk: "high"},
		ApprovalResolved{ApprovalID: "ap1", TaskID: "t1", Decision: "reject", ResolvedBy: "human"},
	})
	if _, err := rejected.Decide(ApproveApproval{ApprovalID: "ap1", ResolvedBy: "other"}); !errors.Is(err, ErrResolved) {
		t.Fatalf("override resolved want ErrResolved, got %v", err)
	}
}
