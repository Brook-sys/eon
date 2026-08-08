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
	ParseStrategyHybrid             ParseStrategy = "hybrid_prefix_positional"
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

// cleanPrefix strips leading markdown list markers (bullets, numbers, quotes)
// and inline emphasis formatting (bold, italic) from a line prefix before
// matching against expected keys (e.g. "- KEY", "* KEY", "1. KEY",
// "**KEY**", "__KEY__", "- **KEY**", "* **KEY**").
func cleanPrefix(prefix string) string {
	s := strings.TrimSpace(prefix)
	for {
		orig := s
		// Strip list markers first, then emphasis markers.
		// List markers: bullet (- , * , +), numbered (1. , 2) , 3: ).
		// Only strip * as bullet when followed by space (distinguishes from italic).
		switch {
		case strings.HasPrefix(s, "- "), strings.HasPrefix(s, "+ "):
			s = strings.TrimSpace(s[2:])
		case strings.HasPrefix(s, "* "):
			s = strings.TrimSpace(s[2:])
		case strings.HasPrefix(s, "• "), strings.HasPrefix(s, "> "):
			s = strings.TrimSpace(s[2:])
		default:
			if idx := strings.IndexAny(s, ".):"); idx > 0 && idx <= 3 {
				isDigits := true
				for _, r := range s[:idx] {
					if r < '0' || r > '9' {
						isDigits = false
						break
					}
				}
				if isDigits {
					s = strings.TrimSpace(s[idx+1:])
				}
			}
		}
		// Strip markdown emphasis markers (**key**, *key*, __key__, _key_).
		s = strings.TrimSpace(s)
		s = stripMarkdownEmphasis(s)
		s = strings.TrimSpace(s)
		if s == orig {
			break
		}
	}
	return s
}

// stripMarkdownEmphasis removes surrounding markdown bold/italic markers
// from a string: **...**, *...*, __...__, _..._. It only strips matched
// pairs at the boundaries to avoid corrupting values containing internal
// asterisks or underscores. The input is expected to be already trimmed
// of leading whitespace.
func stripMarkdownEmphasis(s string) string {
	pairs := []struct{ open, close string }{
		{"**", "**"},
		{"__", "__"},
		{"*", "*"},
		{"_", "_"},
	}
	for _, p := range pairs {
		if len(s) >= len(p.open)+len(p.close) &&
			strings.HasPrefix(s, p.open) && strings.HasSuffix(s, p.close) {
			inner := s[len(p.open) : len(s)-len(p.close)]
			if inner != "" {
				return inner
			}
		}
	}
	return s
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
		prefix := cleanPrefix(trimmed[:colon])
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

	// Collect non-empty lines for fallback analysis.
	nonEmptyCount := 0
	nonEmptyLines := make([]string, 0)
	for _, line := range strings.Split(cleanText, "\n") {
		s := strings.TrimSpace(line)
		if s != "" {
			nonEmptyCount++
			nonEmptyLines = append(nonEmptyLines, s)
		}
	}
	result.NonEmptyLineCount = nonEmptyCount

	if len(result.FoundKeys) == len(keys) {
		// All keys found by prefix — primary strategy.
		result.Strategy = ParseStrategyPrimary
		result.FormatComplianceScore = float64(len(result.FoundKeys)) / float64(len(keys))
	} else if len(result.FoundKeys) == 0 {
		// No keys found by prefix — try pure positional fallback.
		if len(nonEmptyLines) == len(keys) {
			for i, k := range keys {
				result.Values[k] = nonEmptyLines[i]
				result.FoundByFallback = append(result.FoundByFallback, k)
			}
			result.UsedFallback = true
			result.Strategy = ParseStrategyPositionalFallback
			result.FormatComplianceScore = float64(len(result.FoundByFallback)) / float64(len(keys))
		} else {
			result.Strategy = ParseStrategyNone
			result.FormatComplianceScore = 0.0
		}
	} else {
		// Partial prefix match — try hybrid positional fallback for missing keys.
		// Phase 383 adversarial sweep found models sometimes emit one key with
		// prefix and another as a bare value. The hybrid fallback collects
		// non-empty lines that were NOT consumed by prefix matches and assigns
		// them to missing keys in order. Only applies when the count of
		// unmatched non-empty lines equals the count of missing keys.
		missingKeys := make([]string, 0)
		foundSet := make(map[string]bool, len(result.FoundKeys))
		for _, k := range result.FoundKeys {
			foundSet[strings.ToLower(k)] = true
		}
		for _, k := range keys {
			if !foundSet[strings.ToLower(k)] {
				missingKeys = append(missingKeys, k)
			}
		}

		// Collect non-empty lines not consumed by a prefix match.
		consumedLines := make(map[int]bool)
		for _, line := range strings.Split(cleanText, "\n") {
			trimmed := strings.TrimSpace(line)
			colon := strings.Index(trimmed, ":")
			if colon <= 0 {
				continue
			}
			prefix := cleanPrefix(trimmed[:colon])
			for _, k := range keys {
				if strings.EqualFold(prefix, k) {
					// Mark this line index as consumed.
					for i, ne := range nonEmptyLines {
						if ne == trimmed && !consumedLines[i] {
							consumedLines[i] = true
							break
						}
					}
					break
				}
			}
		}

		unmatchedLines := make([]string, 0)
		for i, ne := range nonEmptyLines {
			if !consumedLines[i] {
				unmatchedLines = append(unmatchedLines, ne)
			}
		}

		if len(unmatchedLines) == len(missingKeys) && len(missingKeys) > 0 {
			for i, k := range missingKeys {
				result.Values[k] = unmatchedLines[i]
				result.FoundByFallback = append(result.FoundByFallback, k)
			}
			result.UsedFallback = true
			result.Strategy = ParseStrategyHybrid
			result.FormatComplianceScore = float64(len(result.FoundKeys)+len(result.FoundByFallback)) / float64(len(keys))
		} else {
			// Hybrid fallback not applicable — keep partial primary result.
			result.Strategy = ParseStrategyPrimary
			result.FormatComplianceScore = float64(len(result.FoundKeys)) / float64(len(keys))
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
