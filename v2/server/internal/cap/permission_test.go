package cap

import "testing"

// One assertion per rule in contracts/permission-rules.yaml (spec 17).
func TestPermissionRuleMatrix(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		want Decision
		rule string
	}{
		// Uncoverable set.
		{"U1 explicit deny", Request{Resource: "fs.read", InScope: true, ExplicitDeny: true}, Deny, "U1"},
		{"U2 fs.write outside scope", Request{Resource: "fs.write", OutsideScope: true}, Deny, "U2"},
		{"U3 shell without capability", Request{Resource: "shell", CapabilityPresent: false}, Deny, "U3"},
		{"U4 data_export forced ask", Request{Resource: "data_export"}, Ask, "U4"},

		// Ordered rules.
		{"R1 read in scope", Request{Resource: "fs.read", InScope: true}, Allow, "R1"},
		{"R2 write in scope low risk", Request{Resource: "fs.write", InScope: true, Risk: "low"}, Allow, "R2"},
		{"R3 write in scope high risk", Request{Resource: "fs.write", InScope: true, Risk: "high"}, Ask, "R3"},
		{"R4 net allow-listed", Request{Resource: "net", DomainAllowListed: true}, Allow, "R4"},
		{"R5 net not listed", Request{Resource: "net", DomainAllowListed: false}, Ask, "R5"},
		{"R6 tool with capability", Request{Resource: "tool:search", CapabilityPresent: true}, Allow, "R6"},
		{"R7 default deny (tool without capability)", Request{Resource: "tool:search", CapabilityPresent: false}, Deny, "R7"},
		{"R7 default deny (unknown)", Request{Resource: "fs.read", InScope: false}, Deny, "R7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.req)
			if got.Decision != c.want || got.RuleID != c.rule {
				t.Fatalf("%s: want (%s,%s) got (%s,%s)", c.name, c.want, c.rule, got.Decision, got.RuleID)
			}
		})
	}
}

// Hard denies must not be overridden by an otherwise-allowing context.
func TestUncoverableOverridesAllow(t *testing.T) {
	// fs.write that would be R2-allowed, but outside scope -> U2 hard deny.
	got := Decide(Request{Resource: "fs.write", InScope: true, Risk: "low", OutsideScope: true})
	if got.Decision != Deny || got.RuleID != "U2" {
		t.Fatalf("want (deny,U2) got (%s,%s)", got.Decision, got.RuleID)
	}
}
