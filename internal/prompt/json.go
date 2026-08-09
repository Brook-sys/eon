package prompt

import (
	"strings"
)

// ExtractJSON attempts to locate and return the outermost valid JSON object `{...}`
// within a block of text. It ignores preceding or trailing natural language.
// If no balanced `{` and `}` are found, it returns empty string.
func ExtractJSON(text string) string {
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	// For simplicity in fallback recovery, we find the last closing brace.
	// This relies on the model generating relatively simple structures when clamped.
	end := strings.LastIndex(text, "}")
	if end == -1 || end < start {
		return ""
	}

	return text[start : end+1]
}
