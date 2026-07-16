package modeltext

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeJSONCandidateStripsFenceAndProse(t *testing.T) {
	body := `{"schema_version":1,"id":"c1","mission_revision_id":"m1","operation_id":"o1","base_commit_id":"genesis","read_set":[],"preconditions":[],"changes":[{"kind":"ADD","entity_type":"claim","entity_id":"e1","payload_ref":"p"}],"expected_delta":"d","validator_ids":["schema"],"provenance":"model:x","idempotency_key":"k1"}`
	cases := []struct {
		name string
		raw  string
	}{
		{name: "plain", raw: body},
		{name: "fence json", raw: "```json\n" + body + "\n```"},
		{name: "fence bare", raw: "```\n" + body + "\n```"},
		{name: "prose wrap", raw: "Sure, here is the object:\n" + body + "\nHope that helps!"},
		{name: "bom+fence", raw: "\ufeff```JSON\n" + body + "\n```\n"},
		{name: "trailing fence prose", raw: "```json\n" + body + "\n```\nThanks!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeJSONCandidate(tc.raw)
			if strings.TrimSpace(got.Text) != body {
				t.Fatalf("normalized = %q\nwant %q\napplied=%v", got.Text, body, got.Applied)
			}
			var probe map[string]json.RawMessage
			if err := json.Unmarshal([]byte(got.Text), &probe); err != nil {
				t.Fatalf("result is not JSON: %v", err)
			}
		})
	}
}

func TestNormalizeJSONCandidateDoesNotInventOrMerge(t *testing.T) {
	// Incomplete object — must not invent closing braces.
	raw := `{"a":1`
	got := NormalizeJSONCandidate(raw)
	if got.Text != `{"a":1` && got.Text != raw {
		// After trim only; extraction should fail and leave incomplete text.
		if !strings.HasPrefix(got.Text, `{"a":1`) {
			t.Fatalf("unexpected rewrite of incomplete JSON: %q applied=%v", got.Text, got.Applied)
		}
	}
	if _, ok := extractFirstJSONObject(raw); ok {
		t.Fatal("incomplete object must not extract")
	}

	// Non-JSON remains non-JSON (no hallucination of braces).
	plain := "choose option B"
	got = NormalizeJSONCandidate(plain)
	if strings.Contains(got.Text, "{") {
		t.Fatalf("invented JSON from prose: %q", got.Text)
	}
}

func TestNormalizeJSONCandidateRespectsStringsWithBraces(t *testing.T) {
	body := `{"note":"use } carefully","n":1}`
	raw := "prefix " + body + " suffix"
	got := NormalizeJSONCandidate(raw)
	if got.Text != body {
		t.Fatalf("got %q want %q applied=%v", got.Text, body, got.Applied)
	}
}

func TestNormalizeClosedToken(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "  b  ", want: "B"},
		{raw: "C.", want: "C"},
		{raw: "ANSWER: A", want: "A"},
		{raw: "Opção B", want: "B"},
		{raw: "I think the best is\nB", want: "B"},
		{raw: `"A"`, want: "A"},
		{raw: "option: c!", want: "C"},
	}
	for _, tc := range cases {
		got := NormalizeClosedToken(tc.raw)
		if got.Text != tc.want {
			t.Fatalf("NormalizeClosedToken(%q)=%q want %q applied=%v", tc.raw, got.Text, tc.want, got.Applied)
		}
	}
}

func TestBestJSONCandidate(t *testing.T) {
	body := `{"ok":true}`
	if got := BestJSONCandidate("```json\n" + body + "\n```"); got != body {
		t.Fatalf("BestJSONCandidate = %q", got)
	}
}
