package prompt

import (
	"encoding/json"
	"fmt"
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
	ParseStrategyPrimary                   ParseStrategy = "primary_prefix"
	ParseStrategyPositionalFallback        ParseStrategy = "positional_fallback"
	ParseStrategyPositionalFallbackRelaxed ParseStrategy = "positional_fallback_relaxed"
	ParseStrategyHybrid                    ParseStrategy = "hybrid_prefix_positional"
	ParseStrategyTruncatedPrefix           ParseStrategy = "truncated_prefix"
	ParseStrategyJSONFallback              ParseStrategy = "json_fallback"
	ParseStrategyNone                      ParseStrategy = "none"
)

// ParseResult holds the outcome of parsing a structured response.
//
// Truncated key prefix recovery: when a response is cut short (e.g.
// finish_reason=length), a line may contain an incomplete key prefix like
// "SOUR" instead of "SOURCE". After primary, hybrid, and positional
// strategies are exhausted, the parser attempts truncated prefix matching
// as a last-resort recovery for unmatched keys. This is conservative: it
// only triggers when the cleaned prefix is at least 3 characters and
// matches exactly one known key as a string prefix.
type ParseResult struct {
	// Values maps each requested key to the extracted value. When a key is
	// not found, its value is the empty string.
	Values map[string]string
	// FoundKeys lists keys that were matched by prefix (primary parse).
	FoundKeys []string
	// UsedFallback is true when the primary prefix parse found zero keys
	// and the positional fallback was applied instead.
	UsedFallback bool
	// FoundByTruncated lists keys recovered by truncated prefix matching.
	FoundByTruncated []string
	// FoundByFallback lists keys recovered by the positional fallback.
	FoundByFallback []string
	// Strategy indicates the strategy used to extract values.
	Strategy ParseStrategy
	// FormatComplianceScore is the ratio of extracted keys to total requested keys (0.0 to 1.0).
	FormatComplianceScore float64
	// NonEmptyLineCount is the count of non-empty text lines after normalization.
	NonEmptyLineCount int
}

// cleanPrefix strips leading markdown list markers (bullets, numbers, quotes),
// bracket tags, XML/HTML tags (e.g. <key>, <field_name>), and inline emphasis
// formatting (bold, italic, quotes, backticks, brackets) from a line prefix
// before matching against expected keys (e.g. "- KEY", "* KEY", "1. KEY",
// "**KEY**", "[KEY]", "(KEY)", "<KEY>", "【KEY】", "- **[KEY]**").
func cleanPrefix(prefix string) string {
	s := strings.TrimSpace(prefix)
	for {
		orig := s
		// Strip markdown header hashes (#, ##, ###, etc.)
		if strings.HasPrefix(s, "#") {
			s = strings.TrimLeft(s, "#")
			s = strings.TrimSpace(s)
		}
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
		// Strip bracket tags like [1], (1), [LABEL], (LABEL) followed by space or colon
		if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "(") {
			closeIdx := -1
			if strings.HasPrefix(s, "[") {
				closeIdx = strings.Index(s, "]")
			} else {
				closeIdx = strings.Index(s, ")")
			}
			if closeIdx > 0 && closeIdx < len(s)-1 && (s[closeIdx+1] == ' ' || s[closeIdx+1] == ':') {
				s = strings.TrimSpace(s[closeIdx+1:])
			}
		}
		// Strip markdown emphasis markers (**key**, *key*, __key__, _key_, [key], (key), <key>, 【key】).
		s = strings.TrimSpace(s)
		s = stripMarkdownEmphasis(s)
		s = strings.TrimSpace(s)
		if s == orig {
			break
		}
	}
	return s
}

// stripMarkdownEmphasis removes surrounding markdown bold/italic markers,
// double/single quotes, backticks, brackets, XML/HTML angle brackets, and
// CJK full-width brackets from a string: **...**, *...*, __...__, _..._,
// "... ", '...', `...`, [...], (...), <...>, 【...】. It only strips matched
// pairs at the boundaries to avoid corrupting values containing internal symbols.
// The input is expected to be already trimmed of leading/trailing whitespace.
func stripMarkdownEmphasis(s string) string {
	pairs := []struct{ open, close string }{
		{"**", "**"},
		{"__", "__"},
		{"*", "*"},
		{"_", "_"},
		{"\"", "\""},
		{"'", "'"},
		{"`", "`"},
		{"[", "]"},
		{"(", ")"},
		{"<", ">"},
		{"【", "】"},
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

// extractLinePrefixAndValue parses a line for "KEY: value" or alternate
// separator patterns (" - ", " – ", " — ", " = ", "=", " -> ", " => ", etc.) and returns the cleaned prefix
// and cleaned value. hasKey is true if a valid prefix and separator were found.
func extractLinePrefixAndValue(line string) (prefix string, value string, hasKey bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	trimmed = unescapeHTMLEntities(trimmed)

	var prefStr, valStr string
	if colon := strings.Index(trimmed, ":"); colon > 0 {
		prefStr = trimmed[:colon]
		valStr = trimmed[colon+1:]
	} else if sepIdx, sepLen := findAlternateSeparator(trimmed); sepIdx > 0 {
		prefStr = trimmed[:sepIdx]
		valStr = trimmed[sepIdx+sepLen:]
	} else {
		return "", "", false
	}

	pref := cleanPrefix(prefStr)
	val := cleanValue(valStr)
	return pref, val, true
}

// unescapeHTMLEntities replaces common HTML entity encodings in model text
// before key-value prefix and separator matching.
func unescapeHTMLEntities(s string) string {
	r := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#34;", "\"",
		"&#39;", "'",
		"&apos;", "'",
		"&#58;", ":",
		"&amp;", "&",
	)
	return r.Replace(s)
}

// cleanValue cleans leading value artifacts extracted after separators.
func cleanValue(valStr string) string {
	s := strings.TrimSpace(valStr)
	for {
		orig := s
		switch {
		case strings.HasPrefix(s, "->"):
			s = strings.TrimSpace(s[2:])
		case strings.HasPrefix(s, "=>"):
			s = strings.TrimSpace(s[2:])
		case strings.HasPrefix(s, ">"):
			s = strings.TrimSpace(s[1:])
		case strings.HasPrefix(s, "="):
			s = strings.TrimSpace(s[1:])
		case strings.HasPrefix(s, ":"):
			s = strings.TrimSpace(s[1:])
		case strings.HasPrefix(s, "- "):
			s = strings.TrimSpace(s[2:])
		case strings.HasPrefix(s, "– "):
			s = strings.TrimSpace(s[2:])
		}
		if s == orig {
			break
		}
	}
	return s
}

func findAlternateSeparator(s string) (int, int) {
	seps := []string{
		" - ", " – ", " — ", " = ", "=",
		" -> ", " ->", " => ", " =>",
		":: ", ":=", " : ",
	}
	for _, sep := range seps {
		if idx := strings.Index(s, sep); idx > 0 {
			return idx, len(sep)
		}
	}
	return -1, 0
}

// ParseResponse parses a model response text against an ordered list of
// expected line keys (e.g. []string{"DATE", "SOURCE"}). The primary parse
// looks for lines of the form "KEY: value" (case-insensitive on the key).
// If zero keys are found by prefix, a positional fallback assigns non-empty
// lines to keys in order, but only when the non-empty line count matches the
// key count exactly (avoids false positives on prose or mixed-format
// responses).
//
// Multi-line value folding: when a key is matched on a line, subsequent
// lines that do NOT contain a recognized key prefix are treated as
// continuation lines and appended to the value (joined with a space).
// Folding stops at the next recognized key, a blank line, or a line
// that looks like prose (starts with a capital and ends with a period
// when the continuation is >2 lines). This handles models that wrap
// long values across lines.
func ParseResponse(text string, keys []string) ParseResult {
	result := ParseResult{Values: make(map[string]string, len(keys))}
	if len(keys) == 0 {
		return result
	}
	
	// Phase 411: Try pure JSON unmarshaling first before relying on heuristic text parsing.
	// Many models (especially Qwen/DeepSeek and heavily fine-tuned ones) may emit raw JSON 
	// when format pressure is high, even when asked for simple key-value pairs.
	// Find boundaries of potential JSON object `{...}`.
	firstBrace := strings.Index(text, "{")
	lastBrace := strings.LastIndex(text, "}")
	if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
		potentialJSON := text[firstBrace : lastBrace+1]
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(potentialJSON), &decoded); err == nil {
			// Successfully parsed as JSON object.
			// Verify if it contains the requested keys.
			matchedKeys := 0
			for _, k := range keys {
				if val, ok := decoded[k]; ok {
					// Convert value to string
					strVal := fmt.Sprintf("%v", val)
					// Strip any wrapping quotes from string values if any remain (though Unmarshal normally removes them)
					result.Values[k] = strings.TrimSpace(strVal)
					result.FoundKeys = append(result.FoundKeys, k)
					matchedKeys++
				}
			}
			
			if matchedKeys > 0 {
				result.Strategy = ParseStrategyJSONFallback
				result.FormatComplianceScore = float64(matchedKeys) / float64(len(keys))
				// Add non-empty line count for compatibility (heuristic based on text length)
				result.NonEmptyLineCount = matchedKeys
				return result
			}
			// If JSON parsed but contained NO requested keys, fall through to text parsing.
			// (It might have been JSON from a markdown block that had nothing to do with the requested schema).
			result.FoundKeys = nil
			result.Values = make(map[string]string, len(keys))
		}
	}

	// Normalize text using modeltext ladder (strips BOM, thinking tags, code fences).
	norm := modeltext.NormalizeStructuredResponse(text)
	cleanText := norm.Text

	lowerKeys := make(map[string]string, len(keys))
	for _, k := range keys {
		lowerKeys[strings.ToLower(k)] = ""
	}

	// Build a set of lowercased keys for fast membership testing.
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[strings.ToLower(k)] = true
	}

	// linesForKey checks whether a line's prefix matches any expected key.
	linesForKey := func(line string) (string, bool) {
		prefix, _, ok := extractLinePrefixAndValue(line)
		if !ok {
			return "", false
		}
		if keySet[strings.ToLower(prefix)] {
			return prefix, true
		}
		return "", false
	}

	// bareKeyLines tracks line indices where a bare key name (no separator)
	// was recognized and the value was recovered from a subsequent line.
	bareKeyLines := make(map[int]bool)
	// foldedLines tracks line indices consumed as continuation lines during
	// multi-line value folding, so the hybrid fallback can exclude them from
	// the unmatched-line count.
	foldedLines := make(map[int]bool)
	lines := strings.Split(cleanText, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		prefix, value, ok := extractLinePrefixAndValue(line)
		if !ok {
			// Bare key detection: if the line (after cleaning) is exactly a
			// known key name with no separator, look ahead for the value on
			// the next non-blank line. This handles the pattern where models
			// emit key names on one line and values on the next:
			//   DATE
			//   2026-08-09
			//   SOURCE
			//   Audit Log Beta
			cleaned := strings.TrimSpace(unescapeHTMLEntities(line))
			if cleaned != "" && keySet[strings.ToLower(cleaned)] {
				for _, k := range keys {
					if strings.EqualFold(cleaned, k) && result.Values[k] == "" {
						// Look ahead for value on next non-blank line.
						for lookAhead := i + 1; lookAhead < len(lines); lookAhead++ {
							next := strings.TrimSpace(lines[lookAhead])
							if next == "" {
								continue
							}
							// If next line is another bare key, no value found.
							if _, isKey := linesForKey(next); isKey || (keySet[strings.ToLower(next)] && !strings.Contains(next, ":") && !strings.Contains(next, " - ") && !strings.Contains(next, " =")) {
								break
							}
							result.Values[k] = next
							result.FoundKeys = append(result.FoundKeys, k)
							bareKeyLines[lookAhead] = true
							break
						}
						break
					}
				}
			}
			continue
		}
		// Match against any expected key (case-insensitive).
		matched := false
		for _, k := range keys {
			if strings.EqualFold(prefix, k) {
				if _, exists := result.Values[k]; !exists {
					// Next-line value recovery: if value is empty on line i,
					// look ahead up to 2 lines for a non-empty value line.
					if value == "" {
						for lookAhead := i + 1; lookAhead <= i+2 && lookAhead < len(lines); lookAhead++ {
							nextRaw := lines[lookAhead]
							next := strings.TrimSpace(nextRaw)
							if next == "" {
								continue
							}
							if _, isKey := linesForKey(next); !isKey {
								value = next
								foldedLines[lookAhead] = true
								break
							}
							break
						}
					}
					if value == "" {
						continue
					}
					// Multi-line value folding: collect continuation lines.
					// A line is considered a continuation only if it is indented,
					// or starts with a continuation signal (lowercase letter, comma, semicolon,
					// open bracket/paren, or continuation conjunctions).
					foldedValue := value
					for j := i + 1; j < len(lines); j++ {
						rawLine := lines[j]
						next := strings.TrimSpace(rawLine)
						if next == "" {
							break
						}
						// Stop if the next line contains a recognized key.
						if _, isKey := linesForKey(next); isKey {
							break
						}
						// Stop if the line looks like a key-value pair with a colon
						// but the prefix is not a recognized key.
						if p, _, hasSep := extractLinePrefixAndValue(next); hasSep && !keySet[strings.ToLower(p)] {
							break
						}
						// Check if line is a continuation:
						// 1) Must be indented (starts with space/tab) OR
						// 2) Start with continuation punctuation (, ;) or continuation words (and , or , with ) OR
						// 3) Start with a lowercase letter (standard word wrapping).
						isIndented := strings.HasPrefix(rawLine, " ") || strings.HasPrefix(rawLine, "\t")
						isPunct := strings.HasPrefix(next, ",") || strings.HasPrefix(next, ";")
						lowerNext := strings.ToLower(next)
						isConj := strings.HasPrefix(lowerNext, "and ") || strings.HasPrefix(lowerNext, "or ") || strings.HasPrefix(lowerNext, "with ")
						firstRune := []rune(next)[0]
						isLowercase := firstRune >= 'a' && firstRune <= 'z'

						if !isIndented && !isPunct && !isConj && !isLowercase {
							break
						}

						// Limit folding to 3 continuation lines to avoid runaway prose.
						if j-i > 3 {
							break
						}
						foldedValue += " " + next
						foldedLines[j] = true
					}
					result.Values[k] = foldedValue
					result.FoundKeys = append(result.FoundKeys, k)
				}
				matched = true
				break
			}
		}
		if !matched && len(prefix) >= 3 {
			lowerPref := strings.ToLower(prefix)
			var matchedKey string
			matchCount := 0
			for _, k := range keys {
				if result.Values[k] != "" {
					continue
				}
				if strings.HasPrefix(strings.ToLower(k), lowerPref) {
					matchedKey = k
					matchCount++
				}
			}
			if matchCount == 1 && value != "" {
				result.Values[matchedKey] = value
				result.FoundByTruncated = append(result.FoundByTruncated, matchedKey)
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
		} else if len(nonEmptyLines) > len(keys) && len(nonEmptyLines) <= len(keys)+2 {
			// Relaxed positional fallback: when non-empty line count is slightly higher
			// than key count (1-2 extra lines), but the first N lines are concise (<80 chars),
			// extract positionally from the first N non-empty lines.
			allConcise := true
			for i := 0; i < len(keys); i++ {
				if len(nonEmptyLines[i]) > 80 {
					allConcise = false
					break
				}
			}
			if allConcise {
				for i, k := range keys {
					result.Values[k] = nonEmptyLines[i]
					result.FoundByFallback = append(result.FoundByFallback, k)
				}
				result.UsedFallback = true
				result.Strategy = ParseStrategyPositionalFallbackRelaxed
				result.FormatComplianceScore = float64(len(result.FoundByFallback)) / float64(len(keys))
			} else {
				result.Strategy = ParseStrategyNone
				result.FormatComplianceScore = 0.0
			}
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
		for _, k := range result.FoundByTruncated {
			foundSet[strings.ToLower(k)] = true
		}
		for _, k := range keys {
			if !foundSet[strings.ToLower(k)] {
				missingKeys = append(missingKeys, k)
			}
		}

		// Collect non-empty lines not consumed by a prefix match or folding.
		// Build an index from non-empty line content to the first unconsumed
		// index, so duplicate-looking lines are handled correctly.
		consumedLines := make(map[int]bool)
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			prefix, _, ok := extractLinePrefixAndValue(trimmed)
			if !ok {
				continue
			}
			for _, k := range keys {
				if strings.EqualFold(prefix, k) || (len(prefix) >= 3 && strings.HasPrefix(strings.ToLower(k), strings.ToLower(prefix))) {
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

		// Also mark continuation lines consumed by multi-line folding.
		for lineIdx := range foldedLines {
			foldedText := strings.TrimSpace(lines[lineIdx])
			for i, ne := range nonEmptyLines {
				if ne == foldedText && !consumedLines[i] {
					consumedLines[i] = true
					break
				}
			}
		}

		// Also mark lines consumed as bare-key values.
		for lineIdx := range bareKeyLines {
			bareText := strings.TrimSpace(lines[lineIdx])
			for i, ne := range nonEmptyLines {
				if ne == bareText && !consumedLines[i] {
					consumedLines[i] = true
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
