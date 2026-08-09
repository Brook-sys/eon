package prompt

import (
	"testing"
)

func TestParseScore(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantScore int
		wantFound bool
	}{
		{"clean", "SCORE: 95\nREASON: All good.", 95, true},
		{"lowercase", "score: 42", 42, true},
		{"spaces", "SCORE:    100   ", 100, true},
		{"markdown", "```\nSCORE: 7\n```", 7, true},
		{"embedded", "The final evaluation is SCORE: 85 because of reasons.", 85, true},
		{"missing", "STATUS: SUCCESS", 0, false},
		{"invalid_number", "SCORE: ninety", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScore, gotFound := ParseScore(tt.content)
			if gotScore != tt.wantScore || gotFound != tt.wantFound {
				t.Errorf("ParseScore() = (%v, %v), want (%v, %v)", gotScore, gotFound, tt.wantScore, tt.wantFound)
			}
		})
	}
}
