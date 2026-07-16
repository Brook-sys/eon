package inspect

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"motor-autonomo/internal/domain"
)

// DefaultMaxRawContentBytes caps raw model content returned by inspection APIs.
// The full durable record remains in the store; this is a presentation bound.
const DefaultMaxRawContentBytes = 32 * 1024

const redactedPlaceholder = "[redacted]"

var (
	// Known high-risk free-text patterns. Prefer false negatives over
	// rewriting ordinary prose; ContentHash remains the integrity anchor.
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(authorization)\s*[:=]\s*bearer\s+\S+`),
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/]+=*`),
		regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|secret|password|passwd|credential)\s*[:=]\s*\S+`),
		regexp.MustCompile(`(?i)\b(x-api-key|x-auth-token)\s*[:=]\s*\S+`),
		// OpenAI-style and similar opaque API keys.
		regexp.MustCompile(`\bsk-[A-Za-z0-9]{16,}\b`),
		// Telegram bot tokens: <bot_id>:<secret>
		regexp.MustCompile(`\b\d{8,12}:[A-Za-z0-9_-]{30,}\b`),
		// env NAME=value for common secret names (value only redacted).
		regexp.MustCompile(`(?i)\b([A-Z][A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY|PRIVATE_KEY))\s*=\s*\S+`),
	}
)

// RedactionReport describes presentation-time sanitization of an inspect payload.
type RedactionReport struct {
	Applied        bool     `json:"applied"`
	SecretMatches  int      `json:"secret_matches,omitempty"`
	TruncatedBytes int      `json:"truncated_bytes,omitempty"`
	Notes          []string `json:"notes,omitempty"`
}

// RedactSensitiveText replaces known secret-shaped substrings.
// It is intentionally conservative and never claims completeness.
func RedactSensitiveText(text string) (string, int) {
	if text == "" {
		return text, 0
	}
	out := text
	matches := 0
	for _, re := range secretPatterns {
		loc := re.FindAllStringIndex(out, -1)
		if len(loc) == 0 {
			continue
		}
		matches += len(loc)
		out = re.ReplaceAllString(out, redactedPlaceholder)
	}
	return out, matches
}

// BoundUTF8 truncates s to at most maxBytes without splitting a rune.
// When truncated, a stable ASCII marker is appended and the number of
// removed original bytes is returned.
func BoundUTF8(s string, maxBytes int) (string, int) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, 0
	}
	// Reserve room for the marker.
	marker := "\n…[truncated]"
	limit := maxBytes - len(marker)
	if limit < 0 {
		limit = 0
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	removed := len(s) - cut
	return s[:cut] + marker, removed
}

// RedactRawModelOutput returns a copy safe for operator-facing APIs.
func RedactRawModelOutput(raw domain.RawModelOutput, maxContentBytes int) (domain.RawModelOutput, RedactionReport) {
	if maxContentBytes <= 0 {
		maxContentBytes = DefaultMaxRawContentBytes
	}
	out := raw
	report := RedactionReport{}
	content, n := RedactSensitiveText(out.Content)
	if n > 0 {
		report.Applied = true
		report.SecretMatches += n
		report.Notes = append(report.Notes, "secret-shaped substrings replaced in raw content")
	}
	bounded, removed := BoundUTF8(content, maxContentBytes)
	if removed > 0 {
		report.Applied = true
		report.TruncatedBytes += removed
		report.Notes = append(report.Notes, "raw content truncated for presentation")
	}
	out.Content = bounded
	return out, report
}

// RedactOperationDetail sanitizes free-text evidence fields for export.
// Structural IDs, hashes, and receipts are preserved.
func RedactOperationDetail(detail OperationDetail) (OperationDetail, RedactionReport) {
	out := detail
	report := RedactionReport{}
	if len(out.RawOutputs) == 0 {
		return out, report
	}
	redacted := make([]domain.RawModelOutput, 0, len(out.RawOutputs))
	for _, raw := range out.RawOutputs {
		item, itemReport := RedactRawModelOutput(raw, DefaultMaxRawContentBytes)
		redacted = append(redacted, item)
		mergeRedaction(&report, itemReport)
	}
	out.RawOutputs = redacted
	return out, report
}

// OperationDetailResponse is the operator-facing operation inspector payload.
type OperationDetailResponse struct {
	OperationDetail
	Redaction RedactionReport `json:"redaction"`
}

// CommitDetailResponse is the operator-facing commit inspector payload.
type CommitDetailResponse struct {
	CommitDetail
	Redaction RedactionReport `json:"redaction"`
}

// RedactCommitDetail currently has no free-text model bodies; report is empty.
// Kept for a uniform response envelope and future artifact expansion.
func RedactCommitDetail(detail CommitDetail) (CommitDetail, RedactionReport) {
	return detail, RedactionReport{}
}

func mergeRedaction(dst *RedactionReport, src RedactionReport) {
	if !src.Applied && src.SecretMatches == 0 && src.TruncatedBytes == 0 && len(src.Notes) == 0 {
		return
	}
	dst.Applied = dst.Applied || src.Applied
	dst.SecretMatches += src.SecretMatches
	dst.TruncatedBytes += src.TruncatedBytes
	for _, note := range src.Notes {
		if !containsString(dst.Notes, note) {
			dst.Notes = append(dst.Notes, note)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// NormalizeNote is a tiny helper used by tests for stable assertion text.
func NormalizeNote(s string) string {
	return strings.TrimSpace(s)
}
