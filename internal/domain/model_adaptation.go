package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// AdaptationLevel is the progressive capability ladder for model transport
// enrichment (FR-MODEL-006). Higher levels may improve efficiency or format
// reliability; they MUST NOT expand authority, change OperationSpec semantics,
// or bypass external validation. Baseline text→text remains always available.
type AdaptationLevel string

const (
	// AdaptationBaseline is plain text→text with external parse/validation.
	AdaptationBaseline AdaptationLevel = "BASELINE"
	// AdaptationAssistedJSON requests provider JSON mode when confirmed safe.
	AdaptationAssistedJSON AdaptationLevel = "ASSISTED_JSON"
	// AdaptationNativeTools is reserved for native tool calling. MVP selection
	// never chooses this unless the profile explicitly confirms tools support.
	AdaptationNativeTools AdaptationLevel = "NATIVE_TOOLS"
	// AdaptationExpandedContext uses a larger *safe* context window for fact
	// selection; it never means "send full history" (FR-MODEL-007).
	AdaptationExpandedContext AdaptationLevel = "EXPANDED_CONTEXT"
)

// ContextReductionPolicy controls a reversible ceiling on the compiler window
// after observed provider pressure. It does not mutate the declared profile.
type ContextReductionPolicy struct {
	Active        bool `json:"active"`
	AllowedTokens int  `json:"allowed_tokens"`
}

// ResponseFormatHint is the optional wire enrichment on CompletionRequest.
// Empty means plain text baseline. Only values the kernel understands may be
// selected; adapters must not invent additional authority from unknown hints.
type ResponseFormatHint string

const (
	ResponseFormatNone       ResponseFormatHint = ""
	ResponseFormatJSONObject ResponseFormatHint = "json_object"
)

// ContextBudgetPolicy limits how declared context windows become effective
// compile budgets (FR-MODEL-007). Ratios are in basis points (1/100 of a percent):
// 1250 = 12.5% reserved as safety margin below the declared window.
type ContextBudgetPolicy struct {
	// DeclaredContextTokens is the provider/operator declared window (0 unknown).
	DeclaredContextTokens int `json:"declared_context_tokens,omitempty"`
	// SafeObservedTokens, when > 0, caps the effective window further
	// (empirical safe size). Never exceeds Declared when both set.
	SafeObservedTokens int `json:"safe_observed_tokens,omitempty"`
	// SafetyMarginBps reserves margin below the base window. Default 1250 (12.5%).
	// 0 means use the default; negative is invalid and treated as default.
	SafetyMarginBps int `json:"safety_margin_bps,omitempty"`
	// MaxExpansionBps caps how much of the declared window may be used even
	// when "expanded" adaptation is allowed (default 10000 = 100% of safe base).
	// Values above 10000 are clamped. This never authorizes unbounded history.
	MaxExpansionBps int `json:"max_expansion_bps,omitempty"`
	// AllowExpanded is true when the caller may spend up to MaxExpansionBps of
	// the safe base. When false, an extra conservative reduction applies.
	AllowExpanded bool `json:"allow_expanded,omitempty"`
	// Reduction is an optional dynamic ceiling. Invalid ceilings fail closed.
	Reduction ContextReductionPolicy `json:"reduction,omitempty"`
}

// ApplyReduction returns a copy with a dynamic context ceiling. The declared
// profile remains unchanged, so recovery is an explicit reversible decision.
func (p ContextBudgetPolicy) ApplyReduction(active bool, allowedTokens int) ContextBudgetPolicy {
	p.Reduction = ContextReductionPolicy{Active: active, AllowedTokens: allowedTokens}
	return p
}

// ContextPressureState is a persisted-friendly, authority-free control signal.
// Level grows on confirmed context rejection and recovers only after consecutive
// successes, avoiding oscillation between large and reduced prompts.
type ContextPressureState struct {
	Level            int `json:"level"`
	SuccessesAtLevel int `json:"successes_at_level"`
}

// ModelContextPressure persists the authority-free pressure signal per model
// binding. BindingID is intentionally an opaque configuration ID: no prompt,
// provider body, secret, or policy text is retained in this control record.
type ModelContextPressure struct {
	BindingID string               `json:"binding_id"`
	State     ContextPressureState `json:"state"`
	UpdatedAt time.Time            `json:"updated_at"`
}

func (p ModelContextPressure) Validate() error {
	if strings.TrimSpace(p.BindingID) == "" {
		return errors.New("model context pressure binding id is required")
	}
	if p.State.Level < 0 || p.State.Level > MaxContextPressureLevel {
		return errors.New("model context pressure level is out of range")
	}
	if p.State.SuccessesAtLevel < 0 || p.State.SuccessesAtLevel >= ContextRecoverySuccesses {
		return errors.New("model context pressure success count is out of range")
	}
	if p.State.Level == 0 && p.State.SuccessesAtLevel != 0 {
		return errors.New("zero model context pressure cannot retain successes")
	}
	if p.UpdatedAt.IsZero() {
		return errors.New("model context pressure updated_at is required")
	}
	return nil
}

const (
	MaxContextPressureLevel  = 3
	ContextRecoverySuccesses = 2
)

// RecordContextPressure raises pressure monotonically to the bounded maximum.
func RecordContextPressure(state ContextPressureState) ContextPressureState {
	if state.Level < 0 {
		state.Level = 0
	}
	if state.Level < MaxContextPressureLevel {
		state.Level++
	}
	state.SuccessesAtLevel = 0
	return state
}

// RecordContextSuccess recovers one level only after a stable success streak.
func RecordContextSuccess(state ContextPressureState) ContextPressureState {
	if state.Level <= 0 {
		return ContextPressureState{}
	}
	state.SuccessesAtLevel++
	if state.SuccessesAtLevel >= ContextRecoverySuccesses {
		state.Level--
		state.SuccessesAtLevel = 0
	}
	return state
}

// ReductionForPressure derives a reversible ceiling: each level removes 25%
// of the declared window, with a 25% floor at the maximum pressure level.
func ReductionForPressure(declared int, state ContextPressureState) ContextReductionPolicy {
	if declared <= 0 || state.Level <= 0 {
		return ContextReductionPolicy{}
	}
	level := state.Level
	if level > MaxContextPressureLevel {
		level = MaxContextPressureLevel
	}
	allowed := declared * (4 - level) / 4
	return ContextReductionPolicy{Active: true, AllowedTokens: allowed}
}

// DefaultContextBudgetPolicy returns conservative MVP marks for FR-MODEL-007.
func DefaultContextBudgetPolicy(declared int) ContextBudgetPolicy {
	return ContextBudgetPolicy{
		DeclaredContextTokens: declared,
		SafetyMarginBps:       1250,
		MaxExpansionBps:       10000,
		AllowExpanded:         false,
	}
}

// EffectiveContextTokens returns the conservative token window for prompt
// compilation. Unknown/non-positive declared windows yield 0 (caller must fail
// closed rather than invent capacity).
func (p ContextBudgetPolicy) EffectiveContextTokens() int {
	base := p.DeclaredContextTokens
	if base <= 0 {
		return 0
	}
	if p.SafeObservedTokens > 0 && p.SafeObservedTokens < base {
		base = p.SafeObservedTokens
	}
	marginBps := p.SafetyMarginBps
	if marginBps <= 0 {
		marginBps = 1250
	}
	if marginBps > 9000 {
		// Keep at least 10% of the window usable.
		marginBps = 9000
	}
	// Integer safety: floor((base * (10000-margin)) / 10000)
	usable := base * (10000 - marginBps) / 10000
	if usable < 1 {
		return 0
	}
	expansionBps := p.MaxExpansionBps
	if expansionBps <= 0 {
		expansionBps = 10000
	}
	if expansionBps > 10000 {
		expansionBps = 10000
	}
	if !p.AllowExpanded {
		// Non-expanded path uses at most 87.5% of the already-marginated window
		// (another ~12.5% relative hold-back against optional fact bloat).
		expansionBps = 8750
		if expansionBps > p.MaxExpansionBps && p.MaxExpansionBps > 0 {
			expansionBps = p.MaxExpansionBps
		}
	}
	effective := usable * expansionBps / 10000
	if effective < 1 {
		return 0
	}
	if effective > base {
		effective = base
	}
	if p.Reduction.Active {
		if p.Reduction.AllowedTokens <= 0 {
			return 0
		}
		if effective > p.Reduction.AllowedTokens {
			effective = p.Reduction.AllowedTokens
		}
	}
	return effective
}

// AdaptationPlan is a pure, reversible selection for one Complete attempt.
// It never mutates OperationSpec and never grants capability authority.
type AdaptationPlan struct {
	Level          AdaptationLevel    `json:"level"`
	ResponseFormat ResponseFormatHint `json:"response_format,omitempty"`
	// PrefillAssistant is an optional opening fragment that the adapter emits as
	// a trailing assistant-role message for the model to continue from. It is
	// selected only when the profile explicitly confirms prefill support and the
	// operation contract demands a strict structural opening (FR-MODEL-006;
	// Phase 371 B showed a lone "{" opener eliminates Markdown fences on
	// llama-3.1-8b-instant without trading JSON validity or latency).
	PrefillAssistant string `json:"prefill_assistant,omitempty"`
	// ReasoningEffort is an optional reasoning effort hint (e.g. "none", "low")
	// populated from the provider profile to control internal thinking overhead.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// ContextTokens is the effective compiler window (FR-MODEL-007).
	ContextTokens int `json:"context_tokens,omitempty"`
	// Reason is a short machine-oriented code for audit (no free-form prose).
	Reason string `json:"reason"`
	// Reversible is always true for MVP plans: enrichment may be dropped per call.
	Reversible bool `json:"reversible"`
}

// AdaptationSelectionInput is the authority-free view used to choose a plan.
type AdaptationSelectionInput struct {
	Profile ProviderProfile
	// PreferJSON is true when the operation output contract is JSON-shaped.
	// Preference alone does not enable JSON mode — profile must confirm it.
	PreferJSON bool
	// PreferExpandedContext allows spending more of the safe window on optional facts.
	PreferExpandedContext bool
	// AllowNativeTools is reserved; when false (MVP default), tools are never selected.
	AllowNativeTools bool
	// ContextReduction is an optional dynamic ceiling after observed pressure.
	ContextReduction ContextReductionPolicy
}

// SelectAdaptationPlan chooses the highest *confirmed* enrichment that is safe
// for this call. Unknown/unconfirmed capabilities stay off (FR-MODEL-005/006).
func SelectAdaptationPlan(in AdaptationSelectionInput) AdaptationPlan {
	ctxPolicy := ContextBudgetPolicy{
		DeclaredContextTokens: in.Profile.MaxContextTokens,
		SafetyMarginBps:       1250,
		MaxExpansionBps:       10000,
		AllowExpanded:         in.PreferExpandedContext,
		Reduction:             in.ContextReduction,
	}
	// Prefer SafeObserved-style quirks only when encoded as MaxContextTokens already
	// reduced by the operator; we do not invent a second limit field here.
	effectiveCtx := ctxPolicy.EffectiveContextTokens()

	// Native tools: only when explicitly allowed AND profile confirms support.
	if in.AllowNativeTools && in.Profile.SupportsTools {
		return AdaptationPlan{
			Level:           AdaptationNativeTools,
			ContextTokens:   effectiveCtx,
			ReasoningEffort: in.Profile.DefaultReasoningEffort,
			Reason:          "tools_confirmed",
			Reversible:      true,
		}
	}

	// Assisted JSON only when the profile explicitly confirms JSON mode.
	// Unreliable/unknown must not be presumed available. When prefill is also
	// confirmed, a structural opener is attached (Phase 371 B evidence).
	if in.PreferJSON && in.Profile.SupportsJSONMode {
		level := AdaptationAssistedJSON
		reason := "json_mode_confirmed"
		if in.PreferExpandedContext {
			level = AdaptationExpandedContext
			reason = "json_mode_confirmed_expanded_context"
		}
		plan := AdaptationPlan{
			Level:           level,
			ResponseFormat:  ResponseFormatJSONObject,
			ContextTokens:   effectiveCtx,
			ReasoningEffort: in.Profile.DefaultReasoningEffort,
			Reason:          reason,
			Reversible:      true,
		}
		if in.Profile.SupportsPrefill {
			plan.PrefillAssistant = "{"
		}
		return plan
	}

	// JSON-shaped output without provider JSON mode may still benefit from a
	// confirmed prefill opener; stay at BASELINE level with the prefill hint.
	if in.PreferJSON && in.Profile.SupportsPrefill {
		return AdaptationPlan{
			Level:            AdaptationBaseline,
			PrefillAssistant: "{",
			ContextTokens:    effectiveCtx,
			ReasoningEffort:  in.Profile.DefaultReasoningEffort,
			Reason:           "prefill_confirmed_json_unavailable",
			Reversible:       true,
		}
	}

	if in.PreferExpandedContext && effectiveCtx > 0 {
		return AdaptationPlan{
			Level:           AdaptationExpandedContext,
			ContextTokens:   effectiveCtx,
			ReasoningEffort: in.Profile.DefaultReasoningEffort,
			Reason:          "expanded_context_policy",
			Reversible:      true,
		}
	}

	return AdaptationPlan{
		Level:           AdaptationBaseline,
		ContextTokens:   effectiveCtx,
		ReasoningEffort: in.Profile.DefaultReasoningEffort,
		Reason:          "baseline_text_to_text",
		Reversible:      true,
	}
}

// DemoteAdaptation returns the next safer level after an enrichment failure.
// Demotion is monotonic toward baseline; baseline demotes to itself.
func DemoteAdaptation(level AdaptationLevel) AdaptationLevel {
	switch level {
	case AdaptationNativeTools:
		return AdaptationAssistedJSON
	case AdaptationAssistedJSON, AdaptationExpandedContext:
		return AdaptationBaseline
	default:
		return AdaptationBaseline
	}
}

// PlanAfterDemotion rebuilds a plan for the demoted level using the same profile
// constraints. JSON mode and tools are never re-enabled after demotion unless
// the caller starts a fresh selection with a new profile.
func PlanAfterDemotion(prev AdaptationPlan, profile ProviderProfile) AdaptationPlan {
	next := DemoteAdaptation(prev.Level)
	ctxPolicy := DefaultContextBudgetPolicy(profile.MaxContextTokens)
	// After demotion, never expand context beyond the conservative default.
	ctxPolicy.AllowExpanded = false
	effective := ctxPolicy.EffectiveContextTokens()
	plan := AdaptationPlan{
		Level:         next,
		ContextTokens: effective,
		Reversible:    true,
	}
	switch next {
	case AdaptationAssistedJSON:
		// Only keep JSON mode if still demoting from tools and profile allows.
		if profile.SupportsJSONMode {
			plan.ResponseFormat = ResponseFormatJSONObject
			plan.Reason = "demote_tools_to_json"
			return plan
		}
		plan.Level = AdaptationBaseline
		plan.ResponseFormat = ResponseFormatNone
		plan.Reason = "demote_to_baseline_json_unsupported"
		return plan
	default:
		plan.ResponseFormat = ResponseFormatNone
		plan.Reason = "demote_to_baseline"
		return plan
	}
}

// AdaptationFailureClass is a coarse, non-secret classification for demotion.
type AdaptationFailureClass string

const (
	AdaptationFailureNone            AdaptationFailureClass = ""
	AdaptationFailureFormat          AdaptationFailureClass = "format"
	AdaptationFailureTransportEnrich AdaptationFailureClass = "transport_enrichment"
	AdaptationFailureContext         AdaptationFailureClass = "context"
	AdaptationFailureOther           AdaptationFailureClass = "other"
)

// ShouldDemote reports whether a failed Complete/validation should drop
// enrichment before another model contact. Baseline never demotes further.
func ShouldDemote(level AdaptationLevel, class AdaptationFailureClass) bool {
	if level == AdaptationBaseline || level == "" {
		return false
	}
	switch class {
	case AdaptationFailureFormat, AdaptationFailureTransportEnrich, AdaptationFailureContext:
		return true
	case AdaptationFailureOther:
		// Prefer demotion of unconfirmed enrichment over re-sending enriched.
		return level != AdaptationBaseline
	default:
		return false
	}
}

// ClassifyAdaptationFailure maps a short safe error detail to a class.
// Pure string heuristics only — no model interpretation.
func ClassifyAdaptationFailure(safeDetail string) AdaptationFailureClass {
	s := strings.ToLower(strings.TrimSpace(safeDetail))
	if s == "" {
		return AdaptationFailureOther
	}
	switch {
	case strings.Contains(s, "response_format"),
		strings.Contains(s, "json_object"),
		strings.Contains(s, "json mode"),
		strings.Contains(s, "unsupported"):
		return AdaptationFailureTransportEnrich
	case strings.Contains(s, "context"),
		strings.Contains(s, "token"),
		strings.Contains(s, "too long"),
		strings.Contains(s, "maximum context"):
		return AdaptationFailureContext
	case strings.Contains(s, "json"),
		strings.Contains(s, "parse"),
		strings.Contains(s, "schema"),
		strings.Contains(s, "decode"),
		strings.Contains(s, "invalid"):
		return AdaptationFailureFormat
	default:
		return AdaptationFailureOther
	}
}

// FormatAdaptationAudit returns a compact payload fragment for event PayloadRef.
func FormatAdaptationAudit(plan AdaptationPlan) string {
	return fmt.Sprintf("level=%s;format=%s;ctx=%d;reason=%s;reversible=%t",
		plan.Level, plan.ResponseFormat, plan.ContextTokens, plan.Reason, plan.Reversible)
}
