// Package skillcandidate is the SkillCandidate aggregate: a pure decision core
// (decide) plus event application (apply). It holds no I/O. Invariants
// (spec 13.2, Review-First) are enforced here:
//   - reflection only ever proposes a candidate; nothing is published or
//     applied without an explicit human review command;
//   - a review decision (publish/reject) is terminal and non-overridable.
package skillcandidate

import "errors"

// Domain errors (fail-fast, spec P6). One handling site maps these to codes.
var (
	ErrAlreadyExists = errors.New("skillcandidate: already exists")
	ErrNotFound      = errors.New("skillcandidate: not found")
	ErrReviewed      = errors.New("skillcandidate: already reviewed, cannot override")
)

// Candidate is the aggregate state rebuilt by replaying events.
type Candidate struct {
	exists   bool
	ID       string
	reviewed bool
}

// Apply mutates state for one event (deterministic, no I/O).
func (c *Candidate) Apply(e Event) {
	switch ev := e.(type) {
	case SkillCandidateProposed:
		c.exists = true
		c.ID = ev.CandidateID
	case SkillCandidatePublished:
		c.reviewed = true
	case SkillCandidateRejected:
		c.reviewed = true
	}
}

// Load rebuilds a Candidate from its event history.
func Load(history []Event) *Candidate {
	c := &Candidate{}
	for _, e := range history {
		c.Apply(e)
	}
	return c
}

// Decide validates a command against current state and invariants, returning
// the events to append (pure: no mutation, no I/O).
func (c *Candidate) Decide(cmd Command) ([]Event, error) {
	switch m := cmd.(type) {
	case ProposeSkillCandidate:
		if c.exists {
			return nil, ErrAlreadyExists
		}
		return []Event{SkillCandidateProposed{
			CandidateID: m.CandidateID, Name: m.Name, SourceTaskID: m.SourceTaskID, Summary: m.Summary,
		}}, nil

	case ApproveSkillCandidate:
		if err := c.reviewable(); err != nil {
			return nil, err
		}
		return []Event{SkillCandidatePublished{CandidateID: c.ID, ReviewedBy: m.ReviewedBy}}, nil

	case RejectSkillCandidate:
		if err := c.reviewable(); err != nil {
			return nil, err
		}
		return []Event{SkillCandidateRejected{CandidateID: c.ID, ReviewedBy: m.ReviewedBy, Reason: m.Reason}}, nil

	default:
		return nil, errors.New("skillcandidate: unknown command")
	}
}

// reviewable enforces existence and the non-overridable review invariant.
func (c *Candidate) reviewable() error {
	if !c.exists {
		return ErrNotFound
	}
	if c.reviewed {
		return ErrReviewed
	}
	return nil
}
