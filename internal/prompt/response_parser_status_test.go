package prompt_test

import (
	"testing"

	"motor-autonomo/internal/prompt"
)

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantStatus string
		wantReason string
	}{
		{
			name:       "exact match",
			content:    "STATUS: SUCCESS\nREASON: Transição concluída sem erros.",
			wantStatus: "SUCCESS",
			wantReason: "Transição concluída sem erros.",
		},
		{
			name:       "case insensitive and trailing spaces",
			content:    "status:   FaIlUrE  \nReason: Disk full  ",
			wantStatus: "FAILURE",
			wantReason: "Disk full",
		},
		{
			name:       "markdown fences",
			content:    "```\nSTATUS: SUCCESS\nREASON: All ok\n```",
			wantStatus: "SUCCESS",
			wantReason: "All ok",
		},
		{
			name:       "missing status",
			content:    "REASON: Only reason provided",
			wantStatus: "",
			wantReason: "Only reason provided",
		},
		{
			name:       "missing reason",
			content:    "STATUS: SUCCESS",
			wantStatus: "SUCCESS",
			wantReason: "",
		},
		{
			name:       "other text ignored",
			content:    "Here is the result.\nSTATUS: FAILURE\n\nREASON: Timeout\n\nHope this helps.",
			wantStatus: "FAILURE",
			wantReason: "Timeout",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotReason := prompt.ParseStatus(tc.content)
			if gotStatus != tc.wantStatus {
				t.Errorf("status = %q, want %q", gotStatus, tc.wantStatus)
			}
			if gotReason != tc.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tc.wantReason)
			}
		})
	}
}
