package modeltext

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildShortCorrectionIsLocalized(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 2000)
	got := BuildShortCorrection(ShortCorrectionInput{
		PreviousOutput: long,
		SafeError:      "duplicate key \"id\"",
		AnswerFormat:   "single ProposedChangeSet JSON object",
	})
	if !strings.Contains(got.Prompt, "ERROR: duplicate key") {
		t.Fatalf("missing error: %s", got.Prompt)
	}
	if !strings.Contains(got.Prompt, "REQUIRED_FORMAT:") {
		t.Fatalf("missing format: %s", got.Prompt)
	}
	if strings.Contains(got.Prompt, "operation_id") && strings.Contains(got.Prompt, "mission_revision") {
		// Full fact dumps must not appear — only snippet/format.
	}
	// Must not embed the entire previous output.
	if strings.Count(got.Prompt, "x") > DefaultMaxCorrectionSnippet+10 {
		t.Fatalf("snippet not truncated: prompt len=%d", len(got.Prompt))
	}
	if utf8.RuneCountInString(got.Prompt) > 900 {
		t.Fatalf("correction prompt still too large: %d runes", utf8.RuneCountInString(got.Prompt))
	}
	found := false
	for _, a := range got.Applied {
		if a == "truncate_previous_output" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected truncate tag, applied=%v", got.Applied)
	}
}

func TestBuildShortCorrectionDefaults(t *testing.T) {
	t.Parallel()
	got := BuildShortCorrection(ShortCorrectionInput{})
	if !strings.Contains(got.Prompt, "ERROR:") || !strings.Contains(got.Prompt, "REQUIRED_FORMAT:") {
		t.Fatalf("defaults missing: %s", got.Prompt)
	}
}

func TestBuildSimplerFormatCorrection(t *testing.T) {
	t.Parallel()
	got := BuildSimplerFormatCorrection(`{"bad":true}`, "unknown field")
	if !strings.Contains(got.Prompt, "changes") || !strings.Contains(got.Prompt, "idempotency_key") {
		t.Fatalf("simpler format missing keys: %s", got.Prompt)
	}
	found := false
	for _, a := range got.Applied {
		if a == "simpler_format" {
			found = true
		}
	}
	if !found {
		t.Fatalf("applied=%v", got.Applied)
	}
}
