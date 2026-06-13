package cap

import "strings"

// Decision is the outcome of the permission engine.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

// Request is the normalized input to the permission engine. The HTTP adapter
// derives these flags from the capability, workspace scope, and request params.
type Request struct {
	Resource          string // fs.read | fs.write | net | shell | data_export | tool:<name>
	Risk              string // low | medium | high
	InScope           bool   // target is within the granted workspace scope
	OutsideScope      bool   // target is explicitly outside scope (fs guardrail)
	DomainAllowListed bool   // net target host is allow-listed
	CapabilityPresent bool   // a matching capability token was presented and verified
	ExplicitDeny      bool   // an explicit user/admin deny rule matched
}

// Result is the decision plus the id of the matched rule (for audit/tests).
type Result struct {
	Decision Decision
	RuleID   string
}

// Decide evaluates the permission rule matrix (contracts/permission-rules.yaml)
// deterministically. Order: uncoverable hard-denies (U1-U3) cannot be
// overridden; data_export is forced to ask (U4); then ordered rules R1-R7 with
// first-match-wins and a default deny. The mandatory capability check is woven
// in (U3 denies raw shell without a capability; tool calls need R6's
// capability or fall through to the default deny).
func Decide(r Request) Result {
	// Uncoverable hard denies (spec 11.2): never overridable.
	if r.ExplicitDeny {
		return Result{Deny, "U1"}
	}
	if r.Resource == "fs.write" && r.OutsideScope {
		return Result{Deny, "U2"}
	}
	if r.Resource == "shell" && !r.CapabilityPresent {
		return Result{Deny, "U3"}
	}
	// Uncoverable forced ask: data_export always needs approval + audit.
	if r.Resource == "data_export" {
		return Result{Ask, "U4"}
	}

	// Ordered rules, first match wins.
	switch {
	case r.Resource == "fs.read" && r.InScope:
		return Result{Allow, "R1"}
	case r.Resource == "fs.write" && r.InScope && r.Risk == "low":
		return Result{Allow, "R2"}
	case r.Resource == "fs.write" && r.InScope && (r.Risk == "medium" || r.Risk == "high"):
		return Result{Ask, "R3"}
	case r.Resource == "net" && r.DomainAllowListed:
		return Result{Allow, "R4"}
	case r.Resource == "net" && !r.DomainAllowListed:
		return Result{Ask, "R5"}
	case strings.HasPrefix(r.Resource, "tool:") && r.CapabilityPresent:
		return Result{Allow, "R6"}
	default:
		return Result{Deny, "R7"}
	}
}
