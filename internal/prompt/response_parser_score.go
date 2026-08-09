package prompt

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	scoreRegex = regexp.MustCompile(`(?i)SCORE:\s*(\d+)`)
)

// ParseScore extracts a numeric score from an aggressively clamped model emission.
// It uses a regex anchor SCORE: to bypass formatting and truncation errors.
func ParseScore(content string) (int, bool) {
	match := scoreRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		valStr := strings.TrimSpace(match[1])
		val, err := strconv.Atoi(valStr)
		if err == nil {
			return val, true
		}
	}
	return 0, false
}
