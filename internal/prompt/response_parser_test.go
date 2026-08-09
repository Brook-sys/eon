package prompt

import (
	"strings"
	"testing"
)

func TestParseResponse_PrimaryMatch(t *testing.T) {
	text := "DATE: 2025-11-03\nSOURCE: S-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("should not use fallback when prefix match succeeds")
	}
	if len(r.FoundKeys) != 2 {
		t.Fatalf("expected 2 found keys, got %d", len(r.FoundKeys))
	}
	if got := r.Values["DATE"]; got != "2025-11-03" {
		t.Errorf("DATE=%q, want %q", got, "2025-11-03")
	}
	if got := r.Values["SOURCE"]; got != "S-17" {
		t.Errorf("SOURCE=%q, want %q", got, "S-17")
	}
}

func TestParseResponse_CaseInsensitivePrefix(t *testing.T) {
	text := "date: 2025-11-03\nsource: S-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("should not use fallback")
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q", r.Values["SOURCE"])
	}
}

func TestParseResponse_FallbackBareValues(t *testing.T) {
	// Phase 383 finding: models drop prefix but emit correct values in order.
	text := "2025-11-03\nS-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if !r.UsedFallback {
		t.Fatal("expected fallback to be used")
	}
	if len(r.FoundByFallback) != 2 {
		t.Fatalf("expected 2 fallback keys, got %d", len(r.FoundByFallback))
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q", r.Values["SOURCE"])
	}
}

func TestParseResponse_NoFallbackWhenLineCountExcessive(t *testing.T) {
	// 5 non-empty lines for 2 keys — too many extra lines (> key count + 2), fallback should NOT fire.
	text := "2025-11-03\nS-17\nextra line 1\nextra line 2\nextra line 3"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("fallback must not fire when non-empty line count is excessively greater than key count")
	}
	if len(r.FoundKeys) != 0 {
		t.Fatalf("expected 0 found keys, got %d", len(r.FoundKeys))
	}
}

func TestParseResponse_NoFallbackOnProse(t *testing.T) {
	// Single line of prose, 2 keys — different line count, no fallback.
	text := "The date is 2025-11-03 and the source is S-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("fallback must not fire on prose")
	}
}

func TestParseResponse_PartialMatchHybridFallback(t *testing.T) {
	// Phase 395: When some keys are found by prefix and the remaining
	// non-empty lines match the count of missing keys, hybrid fallback
	// assigns the unmatched lines to missing keys in order.
	text := "DATE: 2025-11-03\nS-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if !r.UsedFallback {
		t.Fatal("hybrid fallback should fire when prefix matches some keys and unmatched lines equal missing keys")
	}
	if r.Strategy != ParseStrategyHybrid {
		t.Fatalf("expected Strategy %q, got %q", ParseStrategyHybrid, r.Strategy)
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q, want S-17", r.Values["SOURCE"])
	}
	if r.FormatComplianceScore != 1.0 {
		t.Errorf("FormatComplianceScore=%f, want 1.0", r.FormatComplianceScore)
	}
}

func TestParseResponse_PartialMatchNoFallbackLineCountMismatch(t *testing.T) {
	// When partial prefix match occurs but unmatched non-empty lines
	// do NOT equal the count of missing keys, hybrid fallback should
	// not fire — keeping the partial primary result.
	text := "DATE: 2025-11-03\nExtra line\nS-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("hybrid fallback must not fire when unmatched line count != missing key count")
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q", r.Values["DATE"])
	}
	if _, ok := r.Values["SOURCE"]; ok {
		t.Error("SOURCE should be missing (line count mismatch prevents hybrid)")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected Strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
}

func TestParseResponse_EmptyText(t *testing.T) {
	r := ParseResponse("", []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("empty text should not trigger fallback")
	}
	if len(r.FoundKeys) != 0 {
		t.Fatalf("expected 0 found keys, got %d", len(r.FoundKeys))
	}
	if r.Strategy != ParseStrategyNone {
		t.Fatalf("expected Strategy to be ParseStrategyNone, got %q", r.Strategy)
	}
	if r.FormatComplianceScore != 0.0 {
		t.Fatalf("expected FormatComplianceScore 0.0, got %f", r.FormatComplianceScore)
	}
}

func TestParseResponse_StrategyAndComplianceScore(t *testing.T) {
	// Primary
	r1 := ParseResponse("DATE: 2025-11-03\nSOURCE: S-17", []string{"DATE", "SOURCE"})
	if r1.Strategy != ParseStrategyPrimary {
		t.Errorf("r1 Strategy=%q, want %q", r1.Strategy, ParseStrategyPrimary)
	}
	if r1.FormatComplianceScore != 1.0 {
		t.Errorf("r1 FormatComplianceScore=%f, want 1.0", r1.FormatComplianceScore)
	}
	if r1.NonEmptyLineCount != 2 {
		t.Errorf("r1 NonEmptyLineCount=%d, want 2", r1.NonEmptyLineCount)
	}

	// Positional Fallback
	r2 := ParseResponse("2025-11-03\nS-17", []string{"DATE", "SOURCE"})
	if r2.Strategy != ParseStrategyPositionalFallback {
		t.Errorf("r2 Strategy=%q, want %q", r2.Strategy, ParseStrategyPositionalFallback)
	}
	if r2.FormatComplianceScore != 1.0 {
		t.Errorf("r2 FormatComplianceScore=%f, want 1.0", r2.FormatComplianceScore)
	}

	// Hybrid (partial prefix + positional for missing)
	r3b := ParseResponse("DATE: 2025-11-03\nS-17", []string{"DATE", "SOURCE"})
	if r3b.Strategy != ParseStrategyHybrid {
		t.Errorf("r3b Strategy=%q, want %q", r3b.Strategy, ParseStrategyHybrid)
	}
	if r3b.FormatComplianceScore != 1.0 {
		t.Errorf("r3b FormatComplianceScore=%f, want 1.0", r3b.FormatComplianceScore)
	}

	// None
	r3 := ParseResponse("random prose text", []string{"DATE", "SOURCE"})
	if r3.Strategy != ParseStrategyNone {
		t.Errorf("r3 Strategy=%q, want %q", r3.Strategy, ParseStrategyNone)
	}
	if r3.FormatComplianceScore != 0.0 {
		t.Errorf("r3 FormatComplianceScore=%f, want 0.0", r3.FormatComplianceScore)
	}
}

func TestParseResponse_EmptyKeys(t *testing.T) {
	r := ParseResponse("anything", []string{})
	if r.UsedFallback {
		t.Fatal("empty keys should not trigger fallback")
	}
	if len(r.Values) != 0 {
		t.Fatalf("expected 0 values, got %d", len(r.Values))
	}
}

func TestParseResponse_ExtraWhitespace(t *testing.T) {
	text := "DATE:   2025-11-03  \n  SOURCE:   S-17  "
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q (whitespace not trimmed)", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q (whitespace not trimmed)", r.Values["SOURCE"])
	}
}

func TestParseResponse_ThreeKeys(t *testing.T) {
	text := "VERDICT: FAIL\nFACTORS: F-4\nEVIDENCE_COUNT: 3"
	r := ParseResponse(text, []string{"VERDICT", "FACTORS", "EVIDENCE_COUNT"})
	if r.UsedFallback {
		t.Fatal("should not use fallback")
	}
	if r.Values["VERDICT"] != "FAIL" {
		t.Errorf("VERDICT=%q", r.Values["VERDICT"])
	}
	if r.Values["FACTORS"] != "F-4" {
		t.Errorf("FACTORS=%q", r.Values["FACTORS"])
	}
	if r.Values["EVIDENCE_COUNT"] != "3" {
		t.Errorf("EVIDENCE_COUNT=%q", r.Values["EVIDENCE_COUNT"])
	}
}

func TestParseResponse_FallbackThreeKeys(t *testing.T) {
	text := "FAIL\nF-4\n3"
	r := ParseResponse(text, []string{"VERDICT", "FACTORS", "EVIDENCE_COUNT"})
	if !r.UsedFallback {
		t.Fatal("expected fallback")
	}
	if r.Values["VERDICT"] != "FAIL" {
		t.Errorf("VERDICT=%q", r.Values["VERDICT"])
	}
	if r.Values["FACTORS"] != "F-4" {
		t.Errorf("FACTORS=%q", r.Values["FACTORS"])
	}
	if r.Values["EVIDENCE_COUNT"] != "3" {
		t.Errorf("EVIDENCE_COUNT=%q", r.Values["EVIDENCE_COUNT"])
	}
}

func TestParseResponse_BlankLinesIgnoredInFallback(t *testing.T) {
	text := "\n2025-11-03\n\nS-17\n"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if !r.UsedFallback {
		t.Fatal("expected fallback (2 non-empty lines == 2 keys)")
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q", r.Values["SOURCE"])
	}
}

func TestParseResponse_DuplicateKeyFirstWins(t *testing.T) {
	text := "DATE: 2025-11-03\nDATE: 2025-12-25\nSOURCE: S-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q, want first occurrence", r.Values["DATE"])
	}
	if len(r.FoundKeys) != 2 {
		t.Fatalf("expected 2 found keys (no dup), got %d", len(r.FoundKeys))
	}
}

func TestParseResponse_EmptyValueSkipped(t *testing.T) {
	text := "DATE: \nSOURCE: S-17"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	// DATE has empty value, so only SOURCE is found by prefix.
	// Since at least one key was found, fallback does NOT fire.
	if r.UsedFallback {
		t.Fatal("fallback must not fire when partial match exists")
	}
	if _, ok := r.Values["DATE"]; ok {
		t.Error("DATE with empty value should not be recorded")
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q", r.Values["SOURCE"])
	}
}

func TestAllMatch_AllCorrect(t *testing.T) {
	r := ParseResponse("DATE: 2025-11-03\nSOURCE: S-17", []string{"DATE", "SOURCE"})
	if !r.AllMatch([]string{"DATE", "SOURCE"}, map[string]string{
		"DATE":   "2025-11-03",
		"SOURCE": "S-17",
	}) {
		t.Fatal("AllMatch should return true when all values match")
	}
}

func TestAllMatch_ValueMismatch(t *testing.T) {
	r := ParseResponse("DATE: 2025-12-25\nSOURCE: S-17", []string{"DATE", "SOURCE"})
	if r.AllMatch([]string{"DATE", "SOURCE"}, map[string]string{
		"DATE":   "2025-11-03",
		"SOURCE": "S-17",
	}) {
		t.Fatal("AllMatch should return false on value mismatch")
	}
}

func TestAllMatch_MissingKey(t *testing.T) {
	r := ParseResponse("DATE: 2025-11-03", []string{"DATE", "SOURCE"})
	if r.AllMatch([]string{"DATE", "SOURCE"}, map[string]string{
		"DATE":   "2025-11-03",
		"SOURCE": "S-17",
	}) {
		t.Fatal("AllMatch should return false when key is missing")
	}
}

func TestParseResponse_FiveKeys(t *testing.T) {
	// Matches the synthesize-factor-trace task with 5 output lines.
	text := "F3: OK\nF4: FAIL\nF5: OK\nVERDICT: FAIL\nFACTORS: F-4"
	r := ParseResponse(text, []string{"F3", "F4", "F5", "VERDICT", "FACTORS"})
	if r.UsedFallback {
		t.Fatal("should not use fallback")
	}
	expected := map[string]string{
		"F3":      "OK",
		"F4":      "FAIL",
		"F5":      "OK",
		"VERDICT": "FAIL",
		"FACTORS": "F-4",
	}
	for k, exp := range expected {
		if r.Values[k] != exp {
			t.Errorf("%s=%q, want %q", k, r.Values[k], exp)
		}
	}
}

func TestParseResponse_FiveKeysFallback(t *testing.T) {
	text := "OK\nFAIL\nOK\nFAIL\nF-4"
	r := ParseResponse(text, []string{"F3", "F4", "F5", "VERDICT", "FACTORS"})
	if !r.UsedFallback {
		t.Fatal("expected fallback for 5 bare values")
	}
	expected := map[string]string{
		"F3":      "OK",
		"F4":      "FAIL",
		"F5":      "OK",
		"VERDICT": "FAIL",
		"FACTORS": "F-4",
	}
	for k, exp := range expected {
		if r.Values[k] != exp {
			t.Errorf("%s=%q, want %q", k, r.Values[k], exp)
		}
	}
}

// TestParseResponse_MarkdownFencesIgnored verifies that markdown code fences
// do not interfere with prefix matching.
func TestParseResponse_MarkdownFencesIgnored(t *testing.T) {
	text := "```\nDATE: 2025-11-03\nSOURCE: S-17\n```"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	// The fence lines have colons but won't match DATE or SOURCE.
	// The actual DATE and SOURCE lines will match by prefix.
	if r.UsedFallback {
		t.Fatal("should not use fallback when keys are found inside fences")
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q", r.Values["SOURCE"])
	}
}

// TestParseResponse_ConflictTask verifies the conflict detection task format.
func TestParseResponse_ConflictTask(t *testing.T) {
	text := "CONFLICT: YES\nPAIR: O-1/O-2"
	r := ParseResponse(text, []string{"CONFLICT", "PAIR"})
	if r.UsedFallback {
		t.Fatal("should not use fallback")
	}
	if r.Values["CONFLICT"] != "YES" {
		t.Errorf("CONFLICT=%q", r.Values["CONFLICT"])
	}
	if r.Values["PAIR"] != "O-1/O-2" {
		t.Errorf("PAIR=%q", r.Values["PAIR"])
	}
}

// TestParseResponse_ConflictTaskFallback verifies fallback for bare conflict values.
func TestParseResponse_ConflictTaskFallback(t *testing.T) {
	text := "YES\nO-1/O-2"
	r := ParseResponse(text, []string{"CONFLICT", "PAIR"})
	if !r.UsedFallback {
		t.Fatal("expected fallback")
	}
	if r.Values["CONFLICT"] != "YES" {
		t.Errorf("CONFLICT=%q", r.Values["CONFLICT"])
	}
	if r.Values["PAIR"] != "O-1/O-2" {
		t.Errorf("PAIR=%q", r.Values["PAIR"])
	}
}

func TestParseResponse_IgnoresThinkingBlocks(t *testing.T) {
	text := "<think>\nThinking about the date: 2020-01-01\nSource could be: S-99\n</think>\nDATE: 2026-08-08\nSOURCE: S-1"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("should match primary keys outside think block")
	}
	if r.Values["DATE"] != "2026-08-08" {
		t.Errorf("DATE=%q, want 2026-08-08", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-1" {
		t.Errorf("SOURCE=%q, want S-1", r.Values["SOURCE"])
	}
}

func TestParseResponse_BulletedMarkdownKeys(t *testing.T) {
	text := "- BUILD: SUCCESS\n* ERRORS: 0_ERRORS\n1. TARGET: PROD"
	r := ParseResponse(text, []string{"BUILD", "ERRORS", "TARGET"})
	if r.UsedFallback {
		t.Fatal("should match primary keys after removing bullet list markers")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["BUILD"] != "SUCCESS" {
		t.Errorf("BUILD=%q, want SUCCESS", r.Values["BUILD"])
	}
	if r.Values["ERRORS"] != "0_ERRORS" {
		t.Errorf("ERRORS=%q, want 0_ERRORS", r.Values["ERRORS"])
	}
	if r.Values["TARGET"] != "PROD" {
		t.Errorf("TARGET=%q, want PROD", r.Values["TARGET"])
	}
}

func TestParseResponse_ThinkingWithMarkdownFences(t *testing.T) {
	text := "<thought>\nLet's analyze\n</thought>\n```text\nDATE: 2026-08-08\nSOURCE: S-42\n```"
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.Values["DATE"] != "2026-08-08" {
		t.Errorf("DATE=%q, want 2026-08-08", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-42" {
		t.Errorf("SOURCE=%q, want S-42", r.Values["SOURCE"])
	}
}

func TestParseResponse_BoldMarkdownKeys(t *testing.T) {
	text := "**BUILD**: SUCCESS\n**ERRORS**: 0_ERRORS\n**TARGET**: PROD"
	r := ParseResponse(text, []string{"BUILD", "ERRORS", "TARGET"})
	if r.UsedFallback {
		t.Fatal("should match primary keys after removing bold markers")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["BUILD"] != "SUCCESS" {
		t.Errorf("BUILD=%q, want SUCCESS", r.Values["BUILD"])
	}
	if r.Values["ERRORS"] != "0_ERRORS" {
		t.Errorf("ERRORS=%q, want 0_ERRORS", r.Values["ERRORS"])
	}
	if r.Values["TARGET"] != "PROD" {
		t.Errorf("TARGET=%q, want PROD", r.Values["TARGET"])
	}
}

func TestParseResponse_ItalicMarkdownKeys(t *testing.T) {
	text := "*BUILD*: SUCCESS\n*ERRORS*: 0_ERRORS"
	r := ParseResponse(text, []string{"BUILD", "ERRORS"})
	if r.UsedFallback {
		t.Fatal("should match primary keys after removing italic markers")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["BUILD"] != "SUCCESS" {
		t.Errorf("BUILD=%q, want SUCCESS", r.Values["BUILD"])
	}
	if r.Values["ERRORS"] != "0_ERRORS" {
		t.Errorf("ERRORS=%q, want 0_ERRORS", r.Values["ERRORS"])
	}
}

func TestParseResponse_UnderscoreBoldKeys(t *testing.T) {
	text := "__BUILD__: SUCCESS\n__ERRORS__: 0_ERRORS"
	r := ParseResponse(text, []string{"BUILD", "ERRORS"})
	if r.UsedFallback {
		t.Fatal("should match primary keys after removing underscore bold markers")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["BUILD"] != "SUCCESS" {
		t.Errorf("BUILD=%q, want SUCCESS", r.Values["BUILD"])
	}
	if r.Values["ERRORS"] != "0_ERRORS" {
		t.Errorf("ERRORS=%q, want 0_ERRORS", r.Values["ERRORS"])
	}
}

func TestParseResponse_MixedBoldAndBulletedKeys(t *testing.T) {
	text := "- **BUILD**: SUCCESS\n* **ERRORS**: 0_ERRORS\n1. **TARGET**: PROD"
	r := ParseResponse(text, []string{"BUILD", "ERRORS", "TARGET"})
	if r.UsedFallback {
		t.Fatal("should match primary keys after removing bullet+bold markers")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["BUILD"] != "SUCCESS" {
		t.Errorf("BUILD=%q, want SUCCESS", r.Values["BUILD"])
	}
	if r.Values["ERRORS"] != "0_ERRORS" {
		t.Errorf("ERRORS=%q, want 0_ERRORS", r.Values["ERRORS"])
	}
	if r.Values["TARGET"] != "PROD" {
		t.Errorf("TARGET=%q, want PROD", r.Values["TARGET"])
	}
}

func TestStripMarkdownEmphasis(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"**BUILD**", "BUILD"},
		{"*BUILD*", "BUILD"},
		{"__BUILD__", "BUILD"},
		{"_BUILD_", "BUILD"},
		{"\"BUILD\"", "BUILD"},
		{"'BUILD'", "BUILD"},
		{"`BUILD`", "BUILD"},
		{"[BUILD]", "BUILD"},
		{"(BUILD)", "BUILD"},
		{"【BUILD】", "BUILD"},
		{"BUILD", "BUILD"},
		{"**", "**"}, // empty inner — not stripped
		{"*", "*"},   // single char — not stripped
		{"", ""},
		{"**B", "**B"}, // unbalanced — not stripped
		{"B**", "B**"}, // unbalanced — not stripped
	}
	for _, c := range cases {
		got := stripMarkdownEmphasis(c.input)
		if got != c.want {
			t.Errorf("stripMarkdownEmphasis(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseResponse_QuotedAndBacktickKeys(t *testing.T) {
	text := "\"BUILD\": SUCCESS\n'ERRORS': 0_ERRORS\n`TARGET`: PROD"
	r := ParseResponse(text, []string{"BUILD", "ERRORS", "TARGET"})
	if r.UsedFallback {
		t.Fatal("should match primary keys after removing quotes and backticks")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["BUILD"] != "SUCCESS" {
		t.Errorf("BUILD=%q, want SUCCESS", r.Values["BUILD"])
	}
	if r.Values["ERRORS"] != "0_ERRORS" {
		t.Errorf("ERRORS=%q, want 0_ERRORS", r.Values["ERRORS"])
	}
	if r.Values["TARGET"] != "PROD" {
		t.Errorf("TARGET=%q, want PROD", r.Values["TARGET"])
	}
}

func TestCleanPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"- KEY", "KEY"},
		{"* KEY", "KEY"},
		{"+ KEY", "KEY"},
		{"1. KEY", "KEY"},
		{"2) KEY", "KEY"},
		{"> KEY", "KEY"},
		{"# KEY", "KEY"},
		{"**KEY**", "KEY"},
		{"*KEY*", "KEY"},
		{"__KEY__", "KEY"},
		{"_KEY_", "KEY"},
		{"- **KEY**", "KEY"},
		{"# KEY", "KEY"},
		{"### **KEY**", "KEY"},
		{"\"KEY\"", "KEY"},
		{"`KEY`", "KEY"},
		{"[KEY]", "KEY"},
		{"(KEY)", "KEY"},
		{"【KEY】", "KEY"},
		{"[1] KEY", "KEY"},
		{"[1] [KEY]", "KEY"},
		{"- [KEY]", "KEY"},
	}
	for _, tc := range tests {
		got := cleanPrefix(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPrefix(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseResponse_PositionalFallbackRelaxed(t *testing.T) {
	// 3 non-empty lines, 2 keys: first 2 lines are short bare values, 3rd line is trailing note.
	text := "2025-11-03\nS-17\nNote: date extracted successfully."
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if !r.UsedFallback {
		t.Fatal("expected relaxed fallback to fire when 3 lines exist for 2 keys and first 2 lines are concise")
	}
	if r.Strategy != ParseStrategyPositionalFallbackRelaxed {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPositionalFallbackRelaxed, r.Strategy)
	}
	if r.Values["DATE"] != "2025-11-03" {
		t.Errorf("DATE=%q, want 2025-11-03", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "S-17" {
		t.Errorf("SOURCE=%q, want S-17", r.Values["SOURCE"])
	}
}

func TestParseResponse_PositionalFallbackRelaxed_LongLine(t *testing.T) {
	// First line is long (>80 chars) prose — should NOT fire relaxed fallback.
	text := "This is a long prose explanation about the date being 2025-11-03 and some more text to exceed 80 chars.\nS-17\nNote."
	r := ParseResponse(text, []string{"DATE", "SOURCE"})
	if r.UsedFallback {
		t.Fatal("relaxed fallback must not fire when any candidate line is long prose")
	}
	if r.Strategy != ParseStrategyNone {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyNone, r.Strategy)
	}
}

func TestParseResponse_BracketAndParenthesesKeys(t *testing.T) {
	text := "[DATE]: 2026-08-08\n(SOURCE): Document A\n【VERDICT】: PASS\n[1] [STATUS]: ACTIVE"
	r := ParseResponse(text, []string{"DATE", "SOURCE", "VERDICT", "STATUS"})
	if r.UsedFallback {
		t.Fatal("should match primary keys with bracket/parentheses prefixes")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["DATE"] != "2026-08-08" {
		t.Errorf("DATE=%q, want 2026-08-08", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Document A" {
		t.Errorf("SOURCE=%q, want Document A", r.Values["SOURCE"])
	}
	if r.Values["VERDICT"] != "PASS" {
		t.Errorf("VERDICT=%q, want PASS", r.Values["VERDICT"])
	}
	if r.Values["STATUS"] != "ACTIVE" {
		t.Errorf("STATUS=%q, want ACTIVE", r.Values["STATUS"])
	}
}

func TestParseResponse_XMLAndHTMLAngleBracketKeys(t *testing.T) {
	text := "<DATE>: 2026-08-08\n<SOURCE>: Audit Document B\n<STATUS>: APPROVED"
	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})
	if r.UsedFallback {
		t.Fatal("should match primary keys with XML/HTML angle brackets")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["DATE"] != "2026-08-08" {
		t.Errorf("DATE=%q, want 2026-08-08", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Audit Document B" {
		t.Errorf("SOURCE=%q, want Audit Document B", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "APPROVED" {
		t.Errorf("STATUS=%q, want APPROVED", r.Values["STATUS"])
	}
}

func TestParseResponse_ArrowAndAssignmentSeparators(t *testing.T) {
	text := "DATE -> 2026-08-08\nSOURCE => Document C\nSTATUS := ACTIVE\nVERDICT :: PASS"
	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS", "VERDICT"})
	if r.UsedFallback {
		t.Fatal("should match primary keys with arrow and assignment separators")
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Fatalf("expected strategy %q, got %q", ParseStrategyPrimary, r.Strategy)
	}
	if r.Values["DATE"] != "2026-08-08" {
		t.Errorf("DATE=%q, want 2026-08-08", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Document C" {
		t.Errorf("SOURCE=%q, want Document C", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "ACTIVE" {
		t.Errorf("STATUS=%q, want ACTIVE", r.Values["STATUS"])
	}
	if r.Values["VERDICT"] != "PASS" {
		t.Errorf("VERDICT=%q, want PASS", r.Values["VERDICT"])
	}
}

func TestParseResponse_MultiLineValueFolding(t *testing.T) {
	// A model emits a value that wraps across lines. The parser should fold
	// continuation lines into the value and still match subsequent keys.
	text := `DESCRIPTION: This is a long description that
spans across multiple lines
for readability
DATE: 2026-08-08
SOURCE: Audit Report`

	r := ParseResponse(text, []string{"DESCRIPTION", "DATE", "SOURCE"})
	if r.Strategy != ParseStrategyPrimary {
		t.Errorf("strategy=%s, want primary_prefix", r.Strategy)
	}
	if r.FormatComplianceScore != 1.0 {
		t.Errorf("compliance=%.2f, want 1.0", r.FormatComplianceScore)
	}
	expectedDesc := "This is a long description that spans across multiple lines for readability"
	if r.Values["DESCRIPTION"] != expectedDesc {
		t.Errorf("DESCRIPTION=%q, want %q", r.Values["DESCRIPTION"], expectedDesc)
	}
	if r.Values["DATE"] != "2026-08-08" {
		t.Errorf("DATE=%q, want 2026-08-08", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "Audit Report" {
		t.Errorf("SOURCE=%q, want Audit Report", r.Values["SOURCE"])
	}
}

func TestParseResponse_MultiLineFoldingStopsAtBlankLine(t *testing.T) {
	// Folding should stop at a blank line and not capture text from the next section.
	text := "TITLE: Short Title\n\nThis is unrelated prose that should not be folded."

	r := ParseResponse(text, []string{"TITLE"})
	if r.Values["TITLE"] != "Short Title" {
		t.Errorf("TITLE=%q, want 'Short Title'", r.Values["TITLE"])
	}
	if r.NonEmptyLineCount != 2 {
		t.Errorf("nonEmptyLines=%d, want 2", r.NonEmptyLineCount)
	}
}

func TestParseResponse_MultiLineFoldingStopsAtUnrecognizedKey(t *testing.T) {
	// If a continuation line has a different key-value pattern with an
	// unrecognized prefix, stop folding.
	text := `SUMMARY: The build completed successfully
NOTES: Additional context here`

	r := ParseResponse(text, []string{"SUMMARY"})
	if r.Values["SUMMARY"] != "The build completed successfully" {
		t.Errorf("SUMMARY=%q, want 'The build completed successfully'", r.Values["SUMMARY"])
	}
}

func TestParseResponse_MultiLineFoldingLimit(t *testing.T) {
	// Folding should stop after 3 continuation lines to avoid runaway prose.
	text := `DETAIL: line0
line1
line2
line3
line4
line5`

	r := ParseResponse(text, []string{"DETAIL"})
	// Should fold at most 3 continuation lines (lines 1-3), stopping at line4.
	expected := "line0 line1 line2 line3"
	if r.Values["DETAIL"] != expected {
		t.Errorf("DETAIL=%q, want %q", r.Values["DETAIL"], expected)
	}
}

func TestParseResponse_HybridFallbackWithFoldedContinuation(t *testing.T) {
	// A model emits a bare value for TITLE (no prefix), then a properly
	// prefixed DESCRIPTION whose value wraps across two lines (indented
	// continuation), then a prefixed SEVERITY. The hybrid fallback must
	// recognize the folded continuation line as consumed so only the bare
	// TITLE line remains as an unmatched candidate.
	text := `Buffer Overflow in Parser
DESCRIPTION: The parser fails to handle input exceeding the allocated buffer size,
              resulting in a buffer overflow and potential code execution.
SEVERITY: HIGH`

	r := ParseResponse(text, []string{"TITLE", "DESCRIPTION", "SEVERITY"})

	// TITLE should be recovered by hybrid fallback.
	if r.Values["TITLE"] != "Buffer Overflow in Parser" {
		t.Errorf("TITLE=%q, want 'Buffer Overflow in Parser'", r.Values["TITLE"])
	}
	// DESCRIPTION should include the folded continuation.
	if !strings.Contains(r.Values["DESCRIPTION"], "resulting in a buffer overflow") {
		t.Errorf("DESCRIPTION=%q, should contain folded continuation", r.Values["DESCRIPTION"])
	}
	// SEVERITY should be HIGH.
	if r.Values["SEVERITY"] != "HIGH" {
		t.Errorf("SEVERITY=%q, want 'HIGH'", r.Values["SEVERITY"])
	}
	// Should use hybrid strategy with full compliance.
	if r.Strategy != ParseStrategyHybrid {
		t.Errorf("Strategy=%v, want %v", r.Strategy, ParseStrategyHybrid)
	}
	if r.FormatComplianceScore != 1.0 {
		t.Errorf("Compliance=%v, want 1.0", r.FormatComplianceScore)
	}
}

func TestParseResponse_NextLineValue(t *testing.T) {
	// Models sometimes emit KEY: on one line and the value on the next line or after a blank line.
	text := `DATE:
2026-08-09
SOURCE:

https://example.com
STATUS: OK`

	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})
	t.Logf("Result: FoundKeys=%v, FoundByFallback=%v, Values=%v, Strategy=%v", r.FoundKeys, r.FoundByFallback, r.Values, r.Strategy)

	if r.Values["DATE"] != "2026-08-09" {
		t.Errorf("DATE=%q, want '2026-08-09'", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "https://example.com" {
		t.Errorf("SOURCE=%q, want 'https://example.com'", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "OK" {
		t.Errorf("STATUS=%q, want 'OK'", r.Values["STATUS"])
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Errorf("Strategy=%v, want %v", r.Strategy, ParseStrategyPrimary)
	}
	if r.FormatComplianceScore != 1.0 {
		t.Errorf("Compliance=%v, want 1.0", r.FormatComplianceScore)
	}
}

func TestParseResponse_HTMLEntityKeysAndSeparators(t *testing.T) {
	// Models sometimes emit HTML-encoded key prefixes or entity separators.
	text := `&lt;DATE&gt;: 2026-08-09
SOURCE&#58; https://example.com
&quot;STATUS&quot;: OK`

	r := ParseResponse(text, []string{"DATE", "SOURCE", "STATUS"})

	if r.Values["DATE"] != "2026-08-09" {
		t.Errorf("DATE=%q, want '2026-08-09'", r.Values["DATE"])
	}
	if r.Values["SOURCE"] != "https://example.com" {
		t.Errorf("SOURCE=%q, want 'https://example.com'", r.Values["SOURCE"])
	}
	if r.Values["STATUS"] != "OK" {
		t.Errorf("STATUS=%q, want 'OK'", r.Values["STATUS"])
	}
	if r.Strategy != ParseStrategyPrimary {
		t.Errorf("Strategy=%v, want %v", r.Strategy, ParseStrategyPrimary)
	}
}
