package evaluation

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/prompt"
)

//go:embed testdata/cognitive-v1.json
var embeddedCognitiveV1 []byte

// OfflineModelLabel is recorded on compile-only reports so they never look like
// a provider baseline.
const OfflineModelLabel = "offline-compile"

// DefaultCognitiveMatrix is the 2k/4k/8k matrix used by the continuity harness
// family and the CLI runner when no overrides are supplied.
func DefaultCognitiveMatrix() Matrix {
	return Matrix{ContextTokens: []int{2048, 4096, 8192}}
}

// LoadEmbeddedCognitiveV1 returns the repository fixture corpus without
// filesystem I/O. Callers still validate via DecodeFixtures.
func LoadEmbeddedCognitiveV1() (FixtureSet, error) {
	return DecodeFixtures(bytes.NewReader(embeddedCognitiveV1), 1<<20)
}

// CompileMatrix exercises the same prompt budget path as Runner.Run but never
// calls a model provider. Each matrix cell records compile success, estimated
// input tokens, and omitted optional facts. Provider/validation fields stay zero.
func CompileMatrix(fixtures FixtureSet, matrix Matrix, estimator prompt.TokenEstimator, spec domain.OperationSpec) (Report, error) {
	if estimator == nil {
		return Report{}, fmt.Errorf("token estimator is required")
	}
	if err := spec.Validate(); err != nil {
		return Report{}, fmt.Errorf("validate benchmark operation spec: %w", err)
	}
	if err := fixtures.Validate(); err != nil {
		return Report{}, err
	}
	if err := matrix.Validate(); err != nil {
		return Report{}, err
	}
	var runs []Run
	for _, c := range fixtures.Cases {
		for _, format := range c.Formats {
			for _, contextTokens := range matrix.ContextTokens {
				run := Run{CaseID: c.ID, Operation: c.Operation, Format: format, ContextTokens: contextTokens}
				input := benchmarkInput(c, format)
				compiled, err := (prompt.Compiler{Estimator: estimator, ProviderContextTokens: contextTokens}).Compile(spec, input)
				if err != nil {
					run.ErrorKind = "COMPILE"
					runs = append(runs, run)
					continue
				}
				run.Compiled = true
				run.EstimatedInputTokens = compiled.EstimatedInputTokens
				run.OmittedFactIDs = compiled.OmittedFactIDs
				runs = append(runs, run)
			}
		}
	}
	report := Report{SchemaVersion: 1, FixtureName: fixtures.Name, Model: OfflineModelLabel, Runs: runs}
	for _, run := range runs {
		report.Summary.Total++
		if run.Compiled {
			report.Summary.Compiled++
		}
		// Compile-only: no syntax/semantic scoring without a model answer.
		report.Summary.InputTokens += run.EstimatedInputTokens
		report.Summary.OmittedFacts += len(run.OmittedFactIDs)
		if run.ErrorKind == "COMPILE" {
			report.Summary.CompileErrors++
		}
	}
	report.Breakdown = summarizeRuns(runs)
	return report, nil
}

// OfflineFindings turns a compile-only report into stable, capped finding lines
// suitable for local continuity audit artifacts.
func OfflineFindings(report Report) []string {
	if report.SchemaVersion != 1 || strings.TrimSpace(report.FixtureName) == "" {
		return []string{"harness:invalid_report"}
	}
	out := []string{
		fmt.Sprintf("harness:fixture=%s", report.FixtureName),
		fmt.Sprintf("harness:model=%s", report.Model),
		fmt.Sprintf("harness:matrix_total=%d", report.Summary.Total),
		fmt.Sprintf("harness:compiled=%d", report.Summary.Compiled),
		fmt.Sprintf("harness:compile_errors=%d", report.Summary.CompileErrors),
		fmt.Sprintf("harness:estimated_input_tokens=%d", report.Summary.InputTokens),
		fmt.Sprintf("harness:omitted_facts=%d", report.Summary.OmittedFacts),
	}
	if report.Summary.CompileErrors == 0 && report.Summary.Compiled == report.Summary.Total && report.Summary.Total > 0 {
		out = append(out, "harness:offline_compile_all_ok")
	}
	// Cap per-context aggregates so audits stay small.
	for i, agg := range report.Breakdown.ByContext {
		if i >= 8 {
			out = append(out, fmt.Sprintf("harness:by_context_truncated=%d", len(report.Breakdown.ByContext)-8))
			break
		}
		out = append(out, fmt.Sprintf("harness:context_%s total=%d compiled=%d errors=%d", agg.Label, agg.Total, agg.Compiled, agg.Errors))
	}
	for i, agg := range report.Breakdown.ByOperation {
		if i >= 8 {
			break
		}
		out = append(out, fmt.Sprintf("harness:operation_%s total=%d compiled=%d errors=%d", agg.Label, agg.Total, agg.Compiled, agg.Errors))
	}
	return out
}
