package prompt

import (
	"regexp"
	"strings"
)

var (
	decisionRegex = regexp.MustCompile(`(?i)DECISION:\s*([A-Za-z0-9_\-]+)`)
)

// ParseDecision extracts a discrete categorical decision (like a multiple-choice option or intent route)
// from an aggressively clamped model emission, using the regex anchor DECISION:.
func ParseDecision(content string) (string, bool) {
	match := decisionRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		val := strings.ToUpper(strings.TrimSpace(match[1]))
		if val != "" {
			return val, true
		}
	}
	return "", false
}
