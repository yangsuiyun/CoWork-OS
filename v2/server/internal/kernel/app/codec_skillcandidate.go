package app

import (
	"encoding/json"
	"fmt"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/events"
	"github.com/coworkos/cowork-os/v2/server/internal/kernel/skillcandidate"
	"github.com/google/uuid"
)

func skillCandidateStreamID(id string) string { return "skillcandidate:" + id }

// decodeSkillCandidateCommand maps a wire command to a SkillCandidate domain
// command and its target stream (camelCase JSON boundary).
func decodeSkillCandidateCommand(cmdType string, payload []byte) (skillcandidate.Command, string, error) {
	switch cmdType {
	case "ProposeSkillCandidate":
		var p struct {
			CandidateID  string `json:"candidateId"`
			Name         string `json:"name"`
			SourceTaskID string `json:"sourceTaskId"`
			Summary      string `json:"summary"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		if p.CandidateID == "" {
			p.CandidateID = uuid.NewString()
		}
		return skillcandidate.ProposeSkillCandidate{CandidateID: p.CandidateID, Name: p.Name, SourceTaskID: p.SourceTaskID, Summary: p.Summary},
			skillCandidateStreamID(p.CandidateID), nil
	case "ApproveSkillCandidate":
		var p struct {
			CandidateID string `json:"candidateId"`
			ReviewedBy  string `json:"reviewedBy"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return skillcandidate.ApproveSkillCandidate{CandidateID: p.CandidateID, ReviewedBy: p.ReviewedBy},
			skillCandidateStreamID(p.CandidateID), nil
	case "RejectSkillCandidate":
		var p struct {
			CandidateID string `json:"candidateId"`
			ReviewedBy  string `json:"reviewedBy"`
			Reason      string `json:"reason"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, "", err
		}
		return skillcandidate.RejectSkillCandidate{CandidateID: p.CandidateID, ReviewedBy: p.ReviewedBy, Reason: p.Reason},
			skillCandidateStreamID(p.CandidateID), nil
	default:
		return nil, "", fmt.Errorf("%w: %s", ErrUnknownCommand, cmdType)
	}
}

// skillCandidateReduce replays the Candidate history, decides, and encodes events.
func skillCandidateReduce(cmd skillcandidate.Command, actor string, history []events.Committed) ([]events.ToAppend, error) {
	domainHist := make([]skillcandidate.Event, 0, len(history))
	for _, c := range history {
		de, err := skillCandidateCommittedToEvent(c)
		if err != nil {
			return nil, err
		}
		domainHist = append(domainHist, de)
	}
	newEvents, err := skillcandidate.Load(domainHist).Decide(cmd) // fail-fast
	if err != nil {
		return nil, err
	}
	out := make([]events.ToAppend, 0, len(newEvents))
	for _, e := range newEvents {
		payload, err := json.Marshal(skillCandidateEventPayload(e))
		if err != nil {
			return nil, err
		}
		out = append(out, events.ToAppend{Type: e.EventType(), SchemaVer: schemaVer, Payload: payload, Actor: actor})
	}
	return out, nil
}

func skillCandidateEventPayload(e skillcandidate.Event) any {
	switch ev := e.(type) {
	case skillcandidate.SkillCandidateProposed:
		m := map[string]any{"candidateId": ev.CandidateID, "name": ev.Name}
		if ev.SourceTaskID != "" {
			m["sourceTaskId"] = ev.SourceTaskID
		}
		if ev.Summary != "" {
			m["summary"] = ev.Summary
		}
		return m
	case skillcandidate.SkillCandidatePublished:
		return map[string]any{"candidateId": ev.CandidateID, "reviewedBy": ev.ReviewedBy}
	case skillcandidate.SkillCandidateRejected:
		m := map[string]any{"candidateId": ev.CandidateID, "reviewedBy": ev.ReviewedBy}
		if ev.Reason != "" {
			m["reason"] = ev.Reason
		}
		return m
	default:
		return map[string]any{}
	}
}

func skillCandidateCommittedToEvent(c events.Committed) (skillcandidate.Event, error) {
	switch c.Type {
	case "SkillCandidateProposed":
		var raw struct {
			CandidateID  string `json:"candidateId"`
			Name         string `json:"name"`
			SourceTaskID string `json:"sourceTaskId"`
			Summary      string `json:"summary"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return skillcandidate.SkillCandidateProposed{CandidateID: raw.CandidateID, Name: raw.Name, SourceTaskID: raw.SourceTaskID, Summary: raw.Summary}, nil
	case "SkillCandidatePublished":
		var raw struct {
			CandidateID string `json:"candidateId"`
			ReviewedBy  string `json:"reviewedBy"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return skillcandidate.SkillCandidatePublished{CandidateID: raw.CandidateID, ReviewedBy: raw.ReviewedBy}, nil
	case "SkillCandidateRejected":
		var raw struct {
			CandidateID string `json:"candidateId"`
			ReviewedBy  string `json:"reviewedBy"`
			Reason      string `json:"reason"`
		}
		if err := json.Unmarshal(c.Payload, &raw); err != nil {
			return nil, err
		}
		return skillcandidate.SkillCandidateRejected{CandidateID: raw.CandidateID, ReviewedBy: raw.ReviewedBy, Reason: raw.Reason}, nil
	default:
		return nil, fmt.Errorf("unknown skillcandidate event type: %s", c.Type)
	}
}
