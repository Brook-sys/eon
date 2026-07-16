package evaluation

import (
	"fmt"
	"sort"
	"strings"
)

// Interpretation is a deterministic, authority-free reading of a cognitive
// benchmark report. It never promotes scores into runtime policy.
type Interpretation struct {
	// Kind is offline-oracle | offline-compile | live | mixed | empty.
	Kind string `json:"kind"`
	// Headline is a single operator-facing sentence.
	Headline string `json:"headline"`
	// Verdict is PASS when every scored run is semantically correct, PARTIAL
	// when some succeed, FAIL when none succeed among scored runs, or
	// UNSCORED when the report has no provider answers.
	Verdict string `json:"verdict"`
	// Notes are stable bullet lines for report.md / continuity audits.
	Notes []string `json:"notes"`
}

// InterpretReport produces a pure interpretation of a completed Report.
// Thresholds are absolute counts only — no ranking of models or policy change.
func InterpretReport(report Report) Interpretation {
	if report.SchemaVersion != 1 || len(report.Runs) == 0 {
		return Interpretation{
			Kind:     "empty",
			Headline: "Report is empty or incomplete; no cognitive baseline can be drawn.",
			Verdict:  "UNSCORED",
			Notes:    []string{"interpret:empty_report"},
		}
	}

	kind := classifyReportKind(report)
	scored := 0
	correct := 0
	syntax := 0
	for _, run := range report.Runs {
		if run.ErrorKind == "COMPILE" {
			continue
		}
		// PROVIDER / VALIDATION still count as scored attempts that failed.
		if run.ErrorKind == "PROVIDER" || run.ErrorKind == "VALIDATION" || run.SyntaxValid || run.SemanticallyCorrect {
			scored++
		}
		if run.SyntaxValid {
			syntax++
		}
		if run.SemanticallyCorrect {
			correct++
		}
	}

	verdict := "UNSCORED"
	switch {
	case scored == 0:
		verdict = "UNSCORED"
	case correct == scored && scored > 0:
		verdict = "PASS"
	case correct == 0:
		verdict = "FAIL"
	default:
		verdict = "PARTIAL"
	}

	notes := []string{
		fmt.Sprintf("interpret:kind=%s", kind),
		fmt.Sprintf("interpret:verdict=%s", verdict),
		fmt.Sprintf("interpret:model=%s", report.Model),
		fmt.Sprintf("interpret:fixture=%s", report.FixtureName),
		fmt.Sprintf("interpret:total=%d", report.Summary.Total),
		fmt.Sprintf("interpret:compiled=%d", report.Summary.Compiled),
		fmt.Sprintf("interpret:syntax_valid=%d", report.Summary.SyntaxValid),
		fmt.Sprintf("interpret:semantically_correct=%d", report.Summary.SemanticallyRight),
		fmt.Sprintf("interpret:errors_compile=%d", report.Summary.CompileErrors),
		fmt.Sprintf("interpret:errors_provider=%d", report.Summary.ProviderErrors),
		fmt.Sprintf("interpret:errors_validation=%d", report.Summary.ValidationErrors),
	}

	// Weakest operation / format slices for operator focus (absolute rates only).
	if worst := weakestAggregate(report.Breakdown.ByOperation); worst != "" {
		notes = append(notes, "interpret:weakest_operation="+worst)
	}
	if worst := weakestAggregate(report.Breakdown.ByFormat); worst != "" {
		notes = append(notes, "interpret:weakest_format="+worst)
	}
	if worst := weakestAggregate(report.Breakdown.ByContext); worst != "" {
		notes = append(notes, "interpret:weakest_context="+worst)
	}

	switch kind {
	case "offline-oracle":
		notes = append(notes, "interpret:oracle_is_harness_ceiling_not_model_skill")
		if verdict == "PASS" {
			notes = append(notes, "interpret:encode_parse_roundtrip_ok")
		} else {
			notes = append(notes, "interpret:encode_parse_roundtrip_broken")
		}
	case "offline-compile":
		notes = append(notes, "interpret:no_provider_answers_scored")
		if report.Summary.CompileErrors == 0 && report.Summary.Compiled == report.Summary.Total {
			notes = append(notes, "interpret:prompt_budget_matrix_compiles")
		}
	case "live":
		notes = append(notes, "interpret:live_provider_baseline")
		if verdict != "PASS" {
			notes = append(notes, "interpret:prefer_weaker_formats_or_smaller_ops_first")
		}
	}

	headline := buildHeadline(kind, verdict, report, correct, scored, syntax)
	return Interpretation{
		Kind:     kind,
		Headline: headline,
		Verdict:  verdict,
		Notes:    notes,
	}
}

func classifyReportKind(report Report) string {
	model := strings.TrimSpace(report.Model)
	switch model {
	case OfflineModelLabel:
		return "offline-compile"
	case OracleModelLabel:
		return "offline-oracle"
	case "":
		// Compile-only runs may leave Model empty when no Complete succeeds.
		if report.Summary.SyntaxValid == 0 && report.Summary.SemanticallyRight == 0 {
			return "offline-compile"
		}
		return "mixed"
	default:
		return "live"
	}
}

func buildHeadline(kind, verdict string, report Report, correct, scored, syntax int) string {
	switch kind {
	case "offline-oracle":
		if verdict == "PASS" {
			return fmt.Sprintf(
				"Offline oracle PASS on %q: %d/%d runs semantically correct (encode→Parse ceiling; not a live model skill).",
				report.FixtureName, correct, report.Summary.Total,
			)
		}
		return fmt.Sprintf(
			"Offline oracle %s on %q: correct=%d syntax=%d total=%d — harness encode/Parse needs attention before live models.",
			verdict, report.FixtureName, correct, syntax, report.Summary.Total,
		)
	case "offline-compile":
		return fmt.Sprintf(
			"Offline compile-only matrix on %q: compiled=%d/%d compile_errors=%d (no provider answers scored).",
			report.FixtureName, report.Summary.Compiled, report.Summary.Total, report.Summary.CompileErrors,
		)
	default:
		return fmt.Sprintf(
			"Cognitive baseline %s for model %q on %q: correct=%d/%d syntax_valid=%d (provider_errors=%d validation_errors=%d).",
			verdict, report.Model, report.FixtureName, correct, scored, syntax,
			report.Summary.ProviderErrors, report.Summary.ValidationErrors,
		)
	}
}

// weakestAggregate returns "label rate=correct/total" for the group with the
// lowest semantic hit rate among groups with Total>0. Ties break by label ASC.
func weakestAggregate(aggs []Aggregate) string {
	type cand struct {
		label   string
		correct int
		total   int
	}
	var best *cand
	for _, a := range aggs {
		if a.Total <= 0 {
			continue
		}
		c := cand{label: a.Label, correct: a.SemanticallyRight, total: a.Total}
		if best == nil {
			best = &c
			continue
		}
		// Compare correct/total without floats: a/b < c/d ⇔ a*d < c*b
		if c.correct*best.total < best.correct*c.total {
			best = &c
			continue
		}
		if c.correct*best.total == best.correct*c.total && c.label < best.label {
			best = &c
		}
	}
	if best == nil {
		return ""
	}
	return fmt.Sprintf("%s rate=%d/%d", best.label, best.correct, best.total)
}

// FormatInterpretationMarkdown appends a short section for report.md.
func FormatInterpretationMarkdown(interp Interpretation) string {
	var b strings.Builder
	b.WriteString("## Interpretation\n\n")
	fmt.Fprintf(&b, "- Kind: `%s`\n", interp.Kind)
	fmt.Fprintf(&b, "- Verdict: `%s`\n", interp.Verdict)
	fmt.Fprintf(&b, "- Headline: %s\n\n", interp.Headline)
	if len(interp.Notes) > 0 {
		b.WriteString("### Notes\n\n")
		// Keep notes sorted for stable diffs when callers shuffle.
		notes := append([]string(nil), interp.Notes...)
		sort.Strings(notes)
		for _, n := range notes {
			fmt.Fprintf(&b, "- `%s`\n", n)
		}
		b.WriteString("\n")
	}
	return b.String()
}
