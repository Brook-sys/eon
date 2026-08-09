package prompt

import (
	"regexp"
	"strings"
)

var (
	statusRegex = regexp.MustCompile(`(?i)STATUS:\s*(SUCCESS|FAILURE)`)
	reasonRegex = regexp.MustCompile(`(?i)REASON:\s*(.+)`)
)

// ParseStatus extracts state transition assertions from aggressively clamped model emissions.
// Phase 438 established that regex-anchored line prefixes survive token compression and PT-BR
// translation significantly better than structured JSON or strict keywords.
func ParseStatus(content string) (status string, reason string) {
	statusMatch := statusRegex.FindStringSubmatch(content)
	if len(statusMatch) > 1 {
		status = strings.ToUpper(strings.TrimSpace(statusMatch[1]))
	}

	reasonMatch := reasonRegex.FindStringSubmatch(content)
	if len(reasonMatch) > 1 {
		reason = strings.TrimSpace(reasonMatch[1])
	}

	return status, reason
}
