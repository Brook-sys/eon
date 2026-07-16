package evaluation

import (
	"strings"
	"testing"

	"motor-autonomo/internal/prompt"
)

func TestLoadEmbeddedCognitiveV1(t *testing.T) {
	t.Parallel()
	set, err := LoadEmbeddedCognitiveV1()
	if err != nil {
		t.Fatal(err)
	}
	if set.Name == "" || len(set.Cases) != 4 {
		t.Fatalf("unexpected embedded fixtures: name=%q cases=%d", set.Name, len(set.Cases))
	}
}

func TestCompileMatrixOffline(t *testing.T) {
	t.Parallel()
	set, err := LoadEmbeddedCognitiveV1()
	if err != nil {
		t.Fatal(err)
	}
	report, err := CompileMatrix(set, DefaultCognitiveMatrix(), prompt.ConservativeEstimator{}, DefaultOperationSpec())
	if err != nil {
		t.Fatal(err)
	}
	if report.Model != OfflineModelLabel {
		t.Fatalf("model label = %q", report.Model)
	}
	if report.Summary.Total == 0 || report.Summary.Compiled != report.Summary.Total {
		t.Fatalf("expected full offline compile success: %+v", report.Summary)
	}
	if report.Summary.CompileErrors != 0 {
		t.Fatalf("compile errors: %d", report.Summary.CompileErrors)
	}
	// 4 cases × 3 formats × 3 contexts = 36 when fixtures use all three formats.
	if report.Summary.Total < 12 {
		t.Fatalf("matrix too small: total=%d", report.Summary.Total)
	}
	findings := OfflineFindings(report)
	joined := strings.Join(findings, "\n")
	for _, needle := range []string{
		"harness:fixture=",
		"harness:model=offline-compile",
		"harness:compiled=",
		"harness:offline_compile_all_ok",
		"harness:context_",
		"harness:operation_",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("findings missing %q in %v", needle, findings)
		}
	}
}
