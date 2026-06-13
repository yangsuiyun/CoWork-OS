package skillcandidate

// Command is a skill-candidate command. CommandType matches the contract name.
type Command interface{ CommandType() string }

// Event is a skill-candidate domain event. EventType matches the catalog type.
type Event interface{ EventType() string }

// --- Commands ---

type ProposeSkillCandidate struct {
	CandidateID  string
	Name         string
	SourceTaskID string
	Summary      string
}

type ApproveSkillCandidate struct {
	CandidateID string
	ReviewedBy  string
}

type RejectSkillCandidate struct {
	CandidateID string
	ReviewedBy  string
	Reason      string
}

func (ProposeSkillCandidate) CommandType() string { return "ProposeSkillCandidate" }
func (ApproveSkillCandidate) CommandType() string { return "ApproveSkillCandidate" }
func (RejectSkillCandidate) CommandType() string  { return "RejectSkillCandidate" }

// --- Events --- (payloads mirror contracts/events/*.schema.json)

type SkillCandidateProposed struct {
	CandidateID  string
	Name         string
	SourceTaskID string
	Summary      string
}

type SkillCandidatePublished struct {
	CandidateID string
	ReviewedBy  string
}

type SkillCandidateRejected struct {
	CandidateID string
	ReviewedBy  string
	Reason      string
}

func (SkillCandidateProposed) EventType() string  { return "SkillCandidateProposed" }
func (SkillCandidatePublished) EventType() string { return "SkillCandidatePublished" }
func (SkillCandidateRejected) EventType() string  { return "SkillCandidateRejected" }
