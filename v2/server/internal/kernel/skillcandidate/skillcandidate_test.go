package skillcandidate

import (
	"errors"
	"testing"
)

func TestProposeThenPublish(t *testing.T) {
	c := Load(nil)

	evs, err := c.Decide(ProposeSkillCandidate{CandidateID: "sc1", Name: "summarize-pr", SourceTaskID: "t1"})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, ok := evs[0].(SkillCandidateProposed); !ok {
		t.Fatalf("expected SkillCandidateProposed, got %T", evs[0])
	}
	c.Apply(evs[0])

	evs, err = c.Decide(ApproveSkillCandidate{CandidateID: "sc1", ReviewedBy: "human"})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	pub, ok := evs[0].(SkillCandidatePublished)
	if !ok || pub.ReviewedBy != "human" {
		t.Fatalf("unexpected publish event: %+v", evs[0])
	}
}

func TestInvariants(t *testing.T) {
	// Duplicate proposal rejected.
	c := Load([]Event{SkillCandidateProposed{CandidateID: "sc1", Name: "x"}})
	if _, err := c.Decide(ProposeSkillCandidate{CandidateID: "sc1", Name: "x"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate propose want ErrAlreadyExists, got %v", err)
	}

	// Review before proposal rejected.
	empty := Load(nil)
	if _, err := empty.Decide(ApproveSkillCandidate{CandidateID: "sc1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("review missing want ErrNotFound, got %v", err)
	}

	// Review-First non-overridable: once rejected, publish is refused.
	rejected := Load([]Event{
		SkillCandidateProposed{CandidateID: "sc1", Name: "x"},
		SkillCandidateRejected{CandidateID: "sc1", ReviewedBy: "human"},
	})
	if _, err := rejected.Decide(ApproveSkillCandidate{CandidateID: "sc1", ReviewedBy: "other"}); !errors.Is(err, ErrReviewed) {
		t.Fatalf("override reviewed want ErrReviewed, got %v", err)
	}
}
