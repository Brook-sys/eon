package prompt

import (
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "pure_json",
			input:    `{"a": 1}`,
			expected: `{"a": 1}`,
		},
		{
			name: "json_in_markdown_block",
			input: "Here is the result:\n```json\n{\n  \"status\": \"SUCCESS\"\n}\n```\nDone.",
			expected: "{\n  \"status\": \"SUCCESS\"\n}",
		},
		{
			name:     "chatter_prefix_and_suffix",
			input:    `Sure! {"decision": "ROUTED"} Have a good day.`,
			expected: `{"decision": "ROUTED"}`,
		},
		{
			name:     "broken_json_syntax", // Note: ExtractJSON just does brace matching, doesn't validate semantics.
			input:    `{"a": }`,
			expected: `{"a": }`,
		},
		{
			name:     "no_braces",
			input:    `DECISION: ROUTED`,
			expected: ``,
		},
		{
			name:     "nested_braces",
			input:    `prefix {"outer": {"inner": true}} suffix`,
			expected: `{"outer": {"inner": true}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractJSON(tc.input)
			if got != tc.expected {
				t.Errorf("ExtractJSON() mismatch\nGot: %q\nExp: %q", got, tc.expected)
			}
		})
	}
}
