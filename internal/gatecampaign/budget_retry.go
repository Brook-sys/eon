package gatecampaign

import (
	"errors"

	"motor-autonomo/internal/port"
)

// ReasoningBudgetExhausted is the non-sensitive diagnostic reason label that
// the OpenAI-compatible adapter emits when finish_reason=length, the semantic
// content is empty, and completion_tokens_details.reasoning_tokens > 0 — i.e.
// the model spent its entire output budget on internal reasoning and produced
// no visible answer. The adapter classifies this to let callers retry with a
// larger budget instead of treating the failure as a permanent semantic error.
const ReasoningBudgetExhausted = "reasoning_budget_exhausted"

// BudgetRetryScale is the multiplier applied to the previous MaxOutputTokens
// when retrying after a reasoning-budget exhaustion. The factor is conservative
// (×4) based on Phase 369/370 live evidence: gpt-oss-20b at max_tokens=8
// consumed all tokens as reasoning; qwen3.6-27b at max_tokens=220 still
// exhausted; recovery required ≥512 for qwen and ≥256 for gpt-oss-20b.
const BudgetRetryScale = 4

// ShouldRetryWithHigherBudget inspects a provider error and the current output
// budget. If the error is a reasoning-budget exhaustion, it returns true and a
// new budget scaled by BudgetRetryScale, capped by maxBudget. If the error is
// anything else, or the scaled budget would not change, it returns false.
//
// The caller is responsible for actually issuing the retry and for enforcing
// its own concurrency, timeout, and total-call limits. This helper is pure and
// deterministic.
func ShouldRetryWithHigherBudget(err error, currentBudget, maxBudget int) (bool, int) {
	if err == nil {
		return false, currentBudget
	}
	var diag port.ProviderDiagnosticError
	if !errors.As(err, &diag) {
		return false, currentBudget
	}
	if diag.DiagnosticReason() != ReasoningBudgetExhausted {
		return false, currentBudget
	}
	scaled := currentBudget * BudgetRetryScale
	if scaled <= currentBudget {
		// overflow or zero budget — no point retrying
		return false, currentBudget
	}
	if maxBudget > 0 && scaled > maxBudget {
		scaled = maxBudget
	}
	if scaled == currentBudget {
		// already at cap
		return false, currentBudget
	}
	return true, scaled
}
