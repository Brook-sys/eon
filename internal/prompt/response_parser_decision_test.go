package prompt

import (
	"testing"
)

func TestParseDecision(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantDec   string
		wantFound bool
	}{
		{"clean", "DECISION: ROUTE_A\nREASON: Matches criteria.", "ROUTE_A", true},
		{"lowercase", "decision: fallback", "FALLBACK", true},
		{"spaces", "DECISION:    ESCALATE   ", "ESCALATE", true},
		{"alphanumeric", "DECISION: Opt123", "OPT123", true},
		{"hyphens", "DECISION: sub-agent-b", "SUB-AGENT-B", true},
		{"missing", "STATUS: SUCCESS", "", false},
		{"empty", "DECISION: ", "", false},
		{"markdown", "```\nDECISION: A\n```", "A", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDec, gotFound := ParseDecision(tt.content)
			if gotDec != tt.wantDec || gotFound != tt.wantFound {
				t.Errorf("ParseDecision() = (%v, %v), want (%v, %v)", gotDec, gotFound, tt.wantDec, tt.wantFound)
			}
		})
	}
}
