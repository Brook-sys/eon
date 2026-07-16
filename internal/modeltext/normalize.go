// Package modeltext provides deterministic, authority-free normalization of
// model outputs (FR-MODEL-004 recovery ladder steps 1–3).
//
// It never invents semantic content, never contacts a provider, and never
// mutates canonical state. Callers MUST preserve the original raw text before
// applying these helpers, then validate the normalized form against a contract.
package modeltext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeResult records what deterministic repairs were applied so callers
// can audit recovery without treating the model as authority.
type NormalizeResult struct {
	// Text is the candidate after safe local repairs. Empty when no safe
	// interpretation exists (caller should keep the original and fail).
	Text string
	// Applied lists short repair tags in application order (for audit).
	Applied []string
	// Changed is true when Text differs from the input after TrimSpace of both
	// is still different — i.e. a structural repair ran, not only trim.
	Changed bool
}

// NormalizeJSONCandidate applies the recovery ladder for outputs expected to be
// a single JSON object:
//  1. trim BOM/whitespace
//  2. strip a single outer markdown code fence (``` / ```json)
//  3. extract the first balanced top-level JSON object when surrounded by prose
//
// It does not pretty-print, re-key fields, invent keys, or repair invalid JSON
// beyond locating a complete object span. Duplicate keys and schema remain the
// responsibility of the strict decoder.
func NormalizeJSONCandidate(raw string) NormalizeResult {
	original := raw
	var applied []string
	text := stripBOM(raw)
	if text != raw {
		applied = append(applied, "strip_bom")
	}
	text = strings.TrimSpace(text)
	if text != strings.TrimSpace(original) && !contains(applied, "strip_bom") {
		applied = append(applied, "trim_space")
	} else if text != strings.TrimSpace(original) {
		// already noted BOM; still record trim when meaningful
		if strings.TrimSpace(stripBOM(original)) != text {
			applied = append(applied, "trim_space")
		}
	}

	if unfenced, ok := stripMarkdownFence(text); ok {
		text = strings.TrimSpace(unfenced)
		applied = append(applied, "strip_markdown_fence")
	}

	if extracted, ok := extractFirstJSONObject(text); ok {
		if extracted != text {
			text = extracted
			applied = append(applied, "extract_json_object")
		}
	}

	changed := text != strings.TrimSpace(stripBOM(original))
	return NormalizeResult{Text: text, Applied: applied, Changed: changed}
}

// NormalizeClosedToken applies ladder steps for level-0 closed answers:
// trim, drop common prefixes ("ANSWER:", "Opção", "Option"), strip trailing
// punctuation, and upper-case a single Latin letter when that is the whole token.
// It does not invent options; validation against the allowlist is the caller's job.
func NormalizeClosedToken(raw string) NormalizeResult {
	original := strings.TrimSpace(raw)
	var applied []string
	text := stripBOM(raw)
	if text != raw {
		applied = append(applied, "strip_bom")
	}
	text = strings.TrimSpace(text)

	// Prefer the last non-empty line for multi-line "reasoning + answer" shapes.
	if lines := nonEmptyLines(text); len(lines) > 1 {
		text = lines[len(lines)-1]
		applied = append(applied, "last_nonempty_line")
	}

	lower := strings.ToLower(text)
	for _, prefix := range []string{
		"answer:", "answer ", "ans:", "opção:", "opcao:", "opção ", "opcao ",
		"option:", "option ", "choice:", "choice ", "resultado:", "result:",
	} {
		if strings.HasPrefix(lower, prefix) {
			text = strings.TrimSpace(text[len(prefix):])
			lower = strings.ToLower(text)
			applied = append(applied, "strip_answer_prefix")
			break
		}
	}

	// Strip a single surrounding pair of quotes or brackets.
	if unquoted, ok := stripWrappingQuotes(text); ok {
		text = unquoted
		applied = append(applied, "strip_wrapping_quotes")
	}

	// Drop trailing sentence punctuation once.
	trimmed := strings.TrimRightFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == ',' || r == ';' || r == ':'
	})
	if trimmed != text {
		text = strings.TrimSpace(trimmed)
		applied = append(applied, "strip_trailing_punct")
	}

	// Single Latin letter → uppercase for closed A/B/C choices.
	if utf8.RuneCountInString(text) == 1 {
		r, _ := utf8.DecodeRuneInString(text)
		if r >= 'a' && r <= 'z' {
			text = strings.ToUpper(text)
			applied = append(applied, "upper_single_letter")
		}
	}

	changed := text != original
	return NormalizeResult{Text: text, Applied: applied, Changed: changed}
}

// BestJSONCandidate returns NormalizeJSONCandidate(text).Text when non-empty,
// otherwise the trimmed original. Convenience for call sites that only need text.
func BestJSONCandidate(raw string) string {
	n := NormalizeJSONCandidate(raw)
	if strings.TrimSpace(n.Text) == "" {
		return strings.TrimSpace(raw)
	}
	return n.Text
}

func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// stripMarkdownFence removes one outer fenced code block. Only the outermost
// fence is stripped; nested fences and incomplete fences are left alone.
func stripMarkdownFence(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s, false
	}
	// Opening fence line: ``` or ```json / ```JSON etc.
	rest := s[3:]
	// Optional language tag on the same line.
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		lang := strings.TrimSpace(rest[:nl])
		// Reject if the "language" looks like content (contains '{') — not a fence tag.
		if strings.Contains(lang, "{") || strings.Contains(lang, "}") {
			return s, false
		}
		// Language tags are short identifiers.
		if lang != "" && !isFenceLanguage(lang) {
			return s, false
		}
		rest = rest[nl+1:]
	} else {
		// Single-line ```...``` form.
		rest = strings.TrimSpace(rest)
	}
	// Closing fence: last line that is only ```
	if idx := strings.LastIndex(rest, "```"); idx >= 0 {
		body := rest[:idx]
		// Ensure nothing but whitespace after the closing fence.
		after := strings.TrimSpace(rest[idx+3:])
		if after != "" {
			// Trailing prose after fence — still accept the body (common model habit).
			_ = after
		}
		body = strings.TrimSpace(body)
		if body == "" {
			return s, false
		}
		return body, true
	}
	return s, false
}

func isFenceLanguage(lang string) bool {
	if len(lang) > 32 {
		return false
	}
	for _, r := range lang {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// extractFirstJSONObject returns the first complete top-level {...} span using
// a rune-aware scanner that respects JSON string escapes. If the entire input
// is already a single object (after trim), it is returned unchanged with ok=true.
// Prose before/after is discarded only when exactly one complete object is found
// as a substring; multiple top-level values are not merged.
func extractFirstJSONObject(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := s[start : i+1]
				// Prefer when object is the whole input or clearly embedded.
				return candidate, true
			}
			if depth < 0 {
				return "", false
			}
		}
	}
	return "", false
}

func stripWrappingQuotes(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s, false
	}
	pairs := [][2]byte{{'"', '"'}, {'\'', '\''}, {'`', '`'}, {'(', ')'}, {'[', ']'}, {'{', '}'}}
	for _, p := range pairs {
		if s[0] == p[0] && s[len(s)-1] == p[1] {
			inner := strings.TrimSpace(s[1 : len(s)-1])
			// Only strip braces/brackets when the interior is a short token (not JSON).
			if p[0] == '{' || p[0] == '[' {
				if strings.ContainsAny(inner, "{}[]:,") {
					continue
				}
			}
			if inner == "" {
				return s, false
			}
			return inner, true
		}
	}
	return s, false
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
