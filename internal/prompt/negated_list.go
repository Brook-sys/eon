package prompt

import (
	"sort"
	"strings"
)

// NegatedSetSelection describes an output line that must list exactly the
// members of a candidate set which failed the gate condition, and nothing
// else.
type NegatedSetSelection struct {
	// LineKey is the exact output key prefix rendered before the colon
	// (e.g. "FACTORS" -> "FACTORS: F-4").
	LineKey string
	// Candidates is the universe of item ids from which failed items are
	// selected. Each candidate's status fact MUST be carried in the prompt
	// facts; this helper only restates the filter rule.
	Candidates []string
	// EmptyToken is rendered when no candidate fails (e.g. "NONE").
	// When empty, the generated instruction states the line reports an
	// empty answer as the empty token, and validates inputs so callers do
	// not silently produce ambiguous contracts.
	EmptyToken string
}

// NegatedListConstraint returns an authority-free constraint string encoding
// the restatement pattern validated live on 2026-08-01
// (heartbeat-2026-08-01-negation-filtering): replacing an implicit negated
// filter ("list items not satisfied") with an explicit restated rule
// ("list ONLY the items that FAILED; satisfied items MUST NOT appear")
// moved Groq/NIM llama-3.1-8b deployments from 0/6 to 6/6 correct on
// multi-factor gate synthesis, while gpt-oss-20b/120b were already 9/9.
//
// The constraint carries no semantic answer: it never names which candidate
// failed; it only restates the filtering contract. Callers build the factual
// per-candidate status lines via Facts as usual.
//
// The second return value reports whether inputs were coherent; when false
// the returned string must be discarded. This helper is deterministic and
// has no provider effect by itself.
func NegatedListConstraint(sel NegatedSetSelection) (string, bool) {
	lineKey := strings.TrimSpace(sel.LineKey)
	if lineKey == "" || strings.ContainsAny(lineKey, ":\n\r\t") {
		return "", false
	}
	candidates := make([]string, 0, len(sel.Candidates))
	seen := make(map[string]struct{}, len(sel.Candidates))
	for _, c := range sel.Candidates {
		c = strings.TrimSpace(c)
		if c == "" || strings.ContainsAny(c, ":\n\r") {
			return "", false
		}
		if _, dup := seen[c]; dup {
			return "", false
		}
		seen[c] = struct{}{}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Strings(candidates)
	empty := strings.TrimSpace(sel.EmptyToken)
	if empty == "" {
		empty = "NONE"
	}
	if strings.ContainsAny(empty, ":\n\r") {
		return "", false
	}
	universe := strings.Join(candidates, ", ")
	return "The " + lineKey + " line lists ONLY the items from {" + universe + "} that FAILED the gate condition. " +
		"Items that are satisfied MUST NOT appear on the " + lineKey + " line, even as context. " +
		"If every item passed, answer " + lineKey + ": " + empty + ". " +
		"Never add commentary, ranges, or items outside {" + universe + "}.", true
}
