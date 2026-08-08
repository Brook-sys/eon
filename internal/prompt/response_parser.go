package prompt

import (
	"strings"

	"motor-autonomo/internal/modeltext"
)

// ResponseParser parses structured KEY: value responses from model completions
// with a format-tolerant fallback. Phase 383 adversarial fire sweep (288 live
// calls across 10 models) found that format compliance is the dominant failure
// mode: models drop the expected "DATE:" / "SOURCE:" prefix but emit correct
// values in the correct order. The parser mirrors the Python sweep runner's
// parse_lines logic so the Go engine can apply the same tolerance.
//
// The parser is deterministic, side-effect free, and never invokes a provider.
// It carries no semantic answer — it only extracts labelled or positional
// values from already-generated text.

// ParseStrategy identifies the parsing technique used to extract values.
type ParseStrategy string

const (
	ParseStrategyPrimary            ParseStrategy = "primary_prefix"
	ParseStrategyPositionalFallback ParseStrategy = "positional_fallback"
	ParseStrategyNone               ParseStrategy = "none"
)

// ParseResult holds the outcome of parsing a structured response.
type ParseResult struct {
	// Values maps each requested key to the extracted value. When a key is
	// not found, its value is the empty string.
	Values map[string]string
	// FoundKeys lists keys that were matched by prefix (primary parse).
	FoundKeys []string
	// UsedFallback is true when the primary prefix parse found zero keys
	// and the positional fallback was applied instead.
	UsedFallback bool
	// FoundByFallback lists keys recovered by the positional fallback.
	FoundByFallback []string
	// Strategy indicates the strategy used to extract values.
	Strategy ParseStrategy
	// FormatComplianceScore is the ratio of extracted keys to total requested keys (0.0 to 1.0).
	FormatComplianceScore float64
	// NonEmptyLineCount is the count of non-empty text lines after normalization.
	NonEmptyLineCount int
}

// ParseResponse parses a model response text against an ordered list of
// expected line keys (e.g. []string{"DATE", "SOURCE"}). The primary parse
// looks for lines of the form "KEY: value" (case-insensitive on the key).
// If zero keys are found by prefix, a positional fallback assigns non-empty
// lines to keys in order, but only when the non-empty line count matches the
// key count exactly (avoids false positives on prose or mixed-format
// responses).
func ParseResponse(text string, keys []string) ParseResult {
	result := ParseResult{Values: make(map[string]string, len(keys))}
	if len(keys) == 0 {
		return result
	}
	// Normalize text using modeltext ladder (strips BOM, thinking tags, code fences).
	norm := modeltext.NormalizeStructuredResponse(text)
	cleanText := norm.Text

	lowerKeys := make(map[string]string, len(keys))
	for _, k := range keys {
		lowerKeys[strings.ToLower(k)] = ""
	}

	// Primary parse: scan lines for "KEY: value" pattern.
	for _, line := range strings.Split(cleanText, "\n") {
		trimmed := strings.TrimSpace(line)
		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			continue
		}
		prefix := strings.TrimSpace(trimmed[:colon])
		value := strings.TrimSpace(trimmed[colon+1:])
		if value == "" {
			continue
		}
		// Match against any expected key (case-insensitive).
		for _, k := range keys {
			if strings.EqualFold(prefix, k) {
				if _, exists := result.Values[k]; !exists {
					result.Values[k] = value
					result.FoundKeys = append(result.FoundKeys, k)
				}
				break
			}
		}
	}

	// Fallback: only when zero keys were found by prefix AND non-empty line
	// count matches key count. This avoids false positives on mixed-format
	// or prose responses.
	nonEmptyCount := 0
	for _, line := range strings.Split(cleanText, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmptyCount++
		}
	}
	result.NonEmptyLineCount = nonEmptyCount

	if len(result.FoundKeys) > 0 {
		result.Strategy = ParseStrategyPrimary
		result.FormatComplianceScore = float64(len(result.FoundKeys)) / float64(len(keys))
	} else if len(result.FoundKeys) == 0 {
		nonEmpty := make([]string, 0, nonEmptyCount)
		for _, line := range strings.Split(cleanText, "\n") {
			s := strings.TrimSpace(line)
			if s != "" {
				nonEmpty = append(nonEmpty, s)
			}
		}
		if len(nonEmpty) == len(keys) {
			for i, k := range keys {
				result.Values[k] = nonEmpty[i]
				result.FoundByFallback = append(result.FoundByFallback, k)
			}
			result.UsedFallback = true
			result.Strategy = ParseStrategyPositionalFallback
			result.FormatComplianceScore = float64(len(result.FoundByFallback)) / float64(len(keys))
		} else {
			result.Strategy = ParseStrategyNone
			result.FormatComplianceScore = 0.0
		}
	}

	return result
}

// AllMatch returns true when every key in keys has a value in result that
// equals the corresponding expected value (case-sensitive, exact match).
// Missing keys count as mismatches.
func (r ParseResult) AllMatch(keys []string, expected map[string]string) bool {
	for _, k := range keys {
		got, ok := r.Values[k]
		if !ok {
			return false
		}
		exp, ok := expected[k]
		if !ok || got != exp {
			return false
		}
	}
	return true
}
