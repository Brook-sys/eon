package modeltext

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// DefaultMaxCorrectionSnippet is the maximum raw model excerpt embedded in a
// short-correction prompt. Keeps step 5 far smaller than a full resend.
const DefaultMaxCorrectionSnippet = 480

// ShortCorrectionInput is the authority-free material for FR-MODEL-004 step 5.
// Callers MUST NOT pass secrets, full system prompts, or capability names.
type ShortCorrectionInput struct {
	// PreviousOutput is the raw or normalized provider text that failed validation.
	PreviousOutput string
	// SafeError is a short, redacted validation reason (no stack traces / bodies).
	SafeError string
	// AnswerFormat restates the expected contract (e.g. single JSON object keys).
	AnswerFormat string
	// MaxSnippet bounds how many runes of PreviousOutput are echoed (0 = default).
	MaxSnippet int
}

// ShortCorrectionResult is the prompt fragment and audit tags.
type ShortCorrectionResult struct {
	Prompt  string
	Applied []string
}

// BuildShortCorrection constructs a localized re-prompt containing only the
// error class, expected format, and a truncated snippet of the prior output.
// It never invents semantic content for the model and never wraps provider APIs.
//
// FR-MODEL-004 / WEAK_MODEL_PROTOCOL: do not re-send the full original prompt.
func BuildShortCorrection(in ShortCorrectionInput) ShortCorrectionResult {
	var applied []string
	max := in.MaxSnippet
	if max <= 0 {
		max = DefaultMaxCorrectionSnippet
	}
	snippet, truncated := truncateRunes(strings.TrimSpace(in.PreviousOutput), max)
	if truncated {
		applied = append(applied, "truncate_previous_output")
	}
	if snippet == "" {
		snippet = "(empty)"
		applied = append(applied, "empty_previous_output")
	}
	errText := strings.TrimSpace(in.SafeError)
	if errText == "" {
		errText = "output failed contract validation"
		applied = append(applied, "default_error")
	}
	if utf8.RuneCountInString(errText) > 240 {
		errText, _ = truncateRunes(errText, 240)
		applied = append(applied, "truncate_error")
	}
	format := strings.TrimSpace(in.AnswerFormat)
	if format == "" {
		format = "exactly one JSON object matching the operation contract; no markdown fences; no prose"
		applied = append(applied, "default_format")
	}

	// Deliberately minimal: error + format + snippet. No task restatement, no facts.
	prompt := fmt.Sprintf(
		"Your previous answer was invalid.\nERROR: %s\nREQUIRED_FORMAT: %s\nPREVIOUS_OUTPUT_SNIPPET:\n%s\nRespond with only the corrected output in REQUIRED_FORMAT. No explanation.",
		errText, format, snippet,
	)
	applied = append(applied, "short_correction_prompt")
	return ShortCorrectionResult{Prompt: prompt, Applied: applied}
}

// SimplerJSONFormat is the reduced answer contract for ladder step 6.
// It drops optional narrative keys and demands the minimal ProposedChangeSet shape.
const SimplerJSONFormat = "single JSON object only with keys: schema_version, id, mission_revision_id, operation_id, base_commit_id, read_set, preconditions, changes, expected_delta, validator_ids, provenance, idempotency_key; no markdown; no prose before or after"

// BuildSimplerFormatCorrection is step 6: short correction plus a stricter/simpler format.
func BuildSimplerFormatCorrection(previousOutput, safeError string) ShortCorrectionResult {
	r := BuildShortCorrection(ShortCorrectionInput{
		PreviousOutput: previousOutput,
		SafeError:      safeError,
		AnswerFormat:   SimplerJSONFormat,
	})
	r.Applied = append(r.Applied, "simpler_format")
	return r
}

func truncateRunes(s string, max int) (string, bool) {
	if max <= 0 || s == "" {
		return s, false
	}
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String() + "…", true
}
