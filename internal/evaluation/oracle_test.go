package evaluation

import (
	"context"
	"os"
	"strings"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
)

func TestEncodeAnswerRoundTrip(t *testing.T) {
	t.Parallel()
	expected := map[string]string{"date": "2025-11-03", "source": "S-17"}
	keys := sortedKeys(expected)
	for _, format := range []Format{FormatChoice, FormatDelimited, FormatJSON} {
		text, err := EncodeAnswer(format, expected)
		if err != nil {
			t.Fatalf("%s encode: %v", format, err)
		}
		got, err := Parse(format, text, keys)
		if err != nil {
			t.Fatalf("%s parse %q: %v", format, text, err)
		}
		if !equalValues(got, expected) {
			t.Fatalf("%s round-trip got=%v want=%v", format, got, expected)
		}
	}
}

func TestRunOraclePerfectCeiling(t *testing.T) {
	t.Parallel()
	set, err := LoadEmbeddedCognitiveV1()
	if err != nil {
		t.Fatal(err)
	}
	matrix := DefaultCognitiveMatrix()
	report, err := RunOracle(context.Background(), set, matrix, prompt.ConservativeEstimator{}, DefaultOperationSpec())
	if err != nil {
		t.Fatal(err)
	}
	if report.Model != OracleModelLabel {
		t.Fatalf("model = %q", report.Model)
	}
	// 4 cases: 3 with 3 formats + repair with 2 formats = 11 format cells × 3 contexts = 33.
	if report.Summary.Total != 33 {
		t.Fatalf("total runs = %d want 33", report.Summary.Total)
	}
	if report.Summary.Compiled != report.Summary.Total {
		t.Fatalf("compiled = %d total = %d", report.Summary.Compiled, report.Summary.Total)
	}
	if report.Summary.SyntaxValid != report.Summary.Total || report.Summary.SemanticallyRight != report.Summary.Total {
		t.Fatalf("oracle ceiling broken: syntax=%d correct=%d total=%d", report.Summary.SyntaxValid, report.Summary.SemanticallyRight, report.Summary.Total)
	}
	if report.Summary.CompileErrors != 0 || report.Summary.ProviderErrors != 0 || report.Summary.ValidationErrors != 0 {
		t.Fatalf("unexpected errors: %+v", report.Summary)
	}
	interp := InterpretReport(report)
	if interp.Kind != "offline-oracle" || interp.Verdict != "PASS" {
		t.Fatalf("interpretation = %+v", interp)
	}
	joined := strings.Join(interp.Notes, "\n")
	if !strings.Contains(joined, "interpret:encode_parse_roundtrip_ok") {
		t.Fatalf("notes missing roundtrip ok: %v", interp.Notes)
	}
}

func TestQueueProviderExhaustion(t *testing.T) {
	t.Parallel()
	p := &QueueProvider{}
	if _, err := p.Complete(context.Background(), port.CompletionRequest{}); err == nil {
		t.Fatal("expected exhaustion error")
	}
}

func TestInterpretLiveReportsEmpiricallyStrongestFormat(t *testing.T) {
	t.Parallel()
	report := Report{
		SchemaVersion: 1,
		FixtureName:   "fixture",
		Model:         "model",
		Summary: Summary{
			Total:             6,
			Compiled:          6,
			SyntaxValid:       4,
			SemanticallyRight: 4,
			ValidationErrors:  2,
		},
		Runs: []Run{
			{SyntaxValid: true, SemanticallyCorrect: true},
			{SyntaxValid: true, SemanticallyCorrect: true},
			{SyntaxValid: true, SemanticallyCorrect: true},
			{SyntaxValid: true, SemanticallyCorrect: true},
			{ErrorKind: "VALIDATION"},
			{ErrorKind: "VALIDATION"},
		},
		Breakdown: Breakdown{ByFormat: []Aggregate{
			{Label: "JSON", Total: 3, SemanticallyRight: 1},
			{Label: "DELIMITED", Total: 3, SemanticallyRight: 3},
		}},
	}

	interp := InterpretReport(report)
	joined := strings.Join(interp.Notes, "\n")
	if !strings.Contains(joined, "interpret:strongest_format=DELIMITED rate=3/3") {
		t.Fatalf("notes missing strongest format: %v", interp.Notes)
	}
	if !strings.Contains(joined, "interpret:weakest_format=JSON rate=1/3") {
		t.Fatalf("notes missing weakest format: %v", interp.Notes)
	}
	if !strings.Contains(joined, "interpret:prefer_empirically_stronger_format_or_smaller_ops_first") {
		t.Fatalf("notes missing evidence-based guidance: %v", interp.Notes)
	}
}

func TestInterpretCompileOnly(t *testing.T) {
	t.Parallel()
	set, err := LoadEmbeddedCognitiveV1()
	if err != nil {
		t.Fatal(err)
	}
	report, err := CompileMatrix(set, Matrix{ContextTokens: []int{2048}}, prompt.ConservativeEstimator{}, DefaultOperationSpec())
	if err != nil {
		t.Fatal(err)
	}
	interp := InterpretReport(report)
	if interp.Kind != "offline-compile" || interp.Verdict != "UNSCORED" {
		t.Fatalf("interpretation = %+v", interp)
	}
}

func TestWriteArtifactsIncludesInterpretation(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	set, err := LoadEmbeddedCognitiveV1()
	if err != nil {
		t.Fatal(err)
	}
	report, err := RunOracle(context.Background(), set, Matrix{ContextTokens: []int{2048}}, prompt.ConservativeEstimator{}, DefaultOperationSpec())
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(directory, report); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(directory + "/report.md")
	if err != nil {
		t.Fatal(err)
	}
	md := string(body)
	if !strings.Contains(md, "## Interpretation") || !strings.Contains(md, "offline-oracle") {
		t.Fatalf("report.md missing interpretation section")
	}
}
