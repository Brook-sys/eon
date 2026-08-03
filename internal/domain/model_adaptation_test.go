package domain

import (
	"strings"
	"testing"
	"time"
)

func TestEffectiveContextTokensConservativeMargins(t *testing.T) {
	t.Parallel()

	// Declared 8192, 12.5% margin → 7168 usable; non-expanded 87.5% of that → 6272.
	p := DefaultContextBudgetPolicy(8192)
	got := p.EffectiveContextTokens()
	want := 8192 * (10000 - 1250) / 10000 * 8750 / 10000
	if got != want {
		t.Fatalf("non-expanded effective = %d, want %d", got, want)
	}

	// Expanded may use full post-margin window.
	p.AllowExpanded = true
	got = p.EffectiveContextTokens()
	want = 8192 * (10000 - 1250) / 10000
	if got != want {
		t.Fatalf("expanded effective = %d, want %d", got, want)
	}

	// Safe observed further caps base before margin.
	p = ContextBudgetPolicy{
		DeclaredContextTokens: 8192,
		SafeObservedTokens:    4096,
		SafetyMarginBps:       1250,
		MaxExpansionBps:       10000,
		AllowExpanded:         true,
	}
	got = p.EffectiveContextTokens()
	want = 4096 * (10000 - 1250) / 10000
	if got != want {
		t.Fatalf("safe-observed expanded = %d, want %d", got, want)
	}

	// Profiles 2k / 8k remain positive and strictly below declared.
	for _, declared := range []int{2048, 8192, 32768} {
		eff := DefaultContextBudgetPolicy(declared).EffectiveContextTokens()
		if eff <= 0 || eff >= declared {
			t.Fatalf("declared=%d effective=%d not strictly conservative", declared, eff)
		}
	}

	if DefaultContextBudgetPolicy(0).EffectiveContextTokens() != 0 {
		t.Fatal("unknown window must yield 0")
	}
}

func TestContextBudgetPolicyReductionAndRecovery(t *testing.T) {
	t.Parallel()

	policy := DefaultContextBudgetPolicy(8192)
	if got := policy.ApplyReduction(true, 4000).EffectiveContextTokens(); got != 4000 {
		t.Fatalf("dynamic ceiling = %d, want 4000", got)
	}
	if got := policy.ApplyReduction(true, -1).EffectiveContextTokens(); got != 0 {
		t.Fatalf("invalid active ceiling must fail closed, got %d", got)
	}

	state := RecordContextPressure(ContextPressureState{})
	if state.Level != 1 || state.SuccessesAtLevel != 0 {
		t.Fatalf("first pressure = %+v", state)
	}
	state = RecordContextPressure(state)
	state = RecordContextPressure(state)
	state = RecordContextPressure(state)
	if state.Level != MaxContextPressureLevel {
		t.Fatalf("pressure must cap at %d: %+v", MaxContextPressureLevel, state)
	}
	reduction := ReductionForPressure(8000, state)
	if !reduction.Active || reduction.AllowedTokens != 2000 {
		t.Fatalf("max pressure reduction = %+v", reduction)
	}

	state = RecordContextSuccess(state)
	if state.Level != MaxContextPressureLevel || state.SuccessesAtLevel != 1 {
		t.Fatalf("single success must not oscillate: %+v", state)
	}
	state = RecordContextSuccess(state)
	if state.Level != MaxContextPressureLevel-1 || state.SuccessesAtLevel != 0 {
		t.Fatalf("success streak must recover one level: %+v", state)
	}

	if reduction := ReductionForPressure(8000, ContextPressureState{}); reduction.Active {
		t.Fatalf("zero pressure unexpectedly active: %+v", reduction)
	}
}

func TestModelContextPressureValidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)
	valid := ModelContextPressure{BindingID: "nim-small", State: ContextPressureState{Level: 2, SuccessesAtLevel: 1}, UpdatedAt: now}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid pressure: %v", err)
	}
	for name, row := range map[string]ModelContextPressure{
		"missing binding":  {State: ContextPressureState{Level: 1}, UpdatedAt: now},
		"level overflow":   {BindingID: "nim-small", State: ContextPressureState{Level: 4}, UpdatedAt: now},
		"invalid streak":   {BindingID: "nim-small", State: ContextPressureState{Level: 1, SuccessesAtLevel: 2}, UpdatedAt: now},
		"zero with streak": {BindingID: "nim-small", State: ContextPressureState{SuccessesAtLevel: 1}, UpdatedAt: now},
		"missing time":     {BindingID: "nim-small", State: ContextPressureState{Level: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := row.Validate(); err == nil {
				t.Fatalf("expected validation error for %+v", row)
			}
		})
	}
}

func TestSelectAdaptationPlanNeverPresumesCapabilities(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	baseline := BaselineDeclaredProfile("local", "tiny", MaxOutputDialectLegacy, 4096, now)
	plan := SelectAdaptationPlan(AdaptationSelectionInput{
		Profile:               baseline,
		PreferJSON:            true,
		PreferExpandedContext: true,
		AllowNativeTools:      true,
	})
	if plan.Level == AdaptationAssistedJSON || plan.ResponseFormat != ResponseFormatNone {
		t.Fatalf("unconfirmed JSON must not be selected: %+v", plan)
	}
	if plan.Level == AdaptationNativeTools {
		t.Fatalf("unconfirmed tools must not be selected: %+v", plan)
	}
	if plan.ContextTokens <= 0 || plan.ContextTokens >= 4096 {
		t.Fatalf("context must be conservative: %+v", plan)
	}
	if !plan.Reversible {
		t.Fatal("plans must be reversible")
	}

	// Confirmed JSON mode.
	rich := baseline
	rich.SupportsJSONMode = true
	rich.Source = CapabilityProbed
	plan = SelectAdaptationPlan(AdaptationSelectionInput{Profile: rich, PreferJSON: true})
	if plan.Level != AdaptationAssistedJSON || plan.ResponseFormat != ResponseFormatJSONObject {
		t.Fatalf("confirmed JSON plan: %+v", plan)
	}

	// Tools only when both allow + support.
	rich.SupportsTools = true
	plan = SelectAdaptationPlan(AdaptationSelectionInput{
		Profile: rich, PreferJSON: true, AllowNativeTools: false,
	})
	if plan.Level == AdaptationNativeTools {
		t.Fatalf("tools blocked by policy: %+v", plan)
	}
	plan = SelectAdaptationPlan(AdaptationSelectionInput{
		Profile: rich, AllowNativeTools: true,
	})
	if plan.Level != AdaptationNativeTools {
		t.Fatalf("tools confirmed: %+v", plan)
	}

	// Reduction is propagated to plan.
	reduced := SelectAdaptationPlan(AdaptationSelectionInput{
		Profile:          baseline,
		ContextReduction: ContextReductionPolicy{Active: true, AllowedTokens: 2000},
	})
	if reduced.ContextTokens != 2000 {
		t.Fatalf("context reduction ignored: %+v", reduced)
	}
}

func TestSelectAdaptationPlanPrefillAssistant(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)

	// Unconfirmed prefill must never emit a prefill fragment, even with
	// confirmed JSON mode and an explicit JSON preference.
	profile := BaselineDeclaredProfile("local", "tiny", MaxOutputDialectLegacy, 4096, now)
	profile.SupportsJSONMode = true
	profile.Source = CapabilityProbed
	plan := SelectAdaptationPlan(AdaptationSelectionInput{Profile: profile, PreferJSON: true})
	if plan.PrefillAssistant != "" {
		t.Fatalf("unconfirmed prefill must stay empty: %+v", plan)
	}

	// Confirmed JSON mode + confirmed prefill attaches the structural opener.
	profile.SupportsPrefill = true
	plan = SelectAdaptationPlan(AdaptationSelectionInput{Profile: profile, PreferJSON: true})
	if plan.Level != AdaptationAssistedJSON || plan.ResponseFormat != ResponseFormatJSONObject {
		t.Fatalf("confirmed JSON plan: %+v", plan)
	}
	if plan.PrefillAssistant != "{" {
		t.Fatalf("prefill opener expected: %+v", plan)
	}

	// JSON-mode unavailable + confirmed prefill stays at baseline with the
	// structural opener instead of fencing cleanup downstream.
	plain := BaselineDeclaredProfile("local", "tiny", MaxOutputDialectLegacy, 4096, now)
	plain.SupportsPrefill = true
	plain.Source = CapabilityProbed
	plan = SelectAdaptationPlan(AdaptationSelectionInput{Profile: plain, PreferJSON: true})
	if plan.Level != AdaptationBaseline || plan.ResponseFormat != ResponseFormatNone {
		t.Fatalf("prefill without JSON mode stays baseline: %+v", plan)
	}
	if plan.PrefillAssistant != "{" || plan.Reason != "prefill_confirmed_json_unavailable" {
		t.Fatalf("prefill baseline plan: %+v", plan)
	}

	// Demotion always clears prefill: after an enrichment failure the kernel
	// must fall back to the plainest contract still confirmed.
	prev := AdaptationPlan{Level: AdaptationAssistedJSON, ResponseFormat: ResponseFormatJSONObject, PrefillAssistant: "{", Reason: "json_mode_confirmed"}
	next := PlanAfterDemotion(prev, profile)
	if next.PrefillAssistant != "" {
		t.Fatalf("demotion must clear prefill: %+v", next)
	}
}

func TestDemoteAdaptationAndShouldDemote(t *testing.T) {
	t.Parallel()

	if DemoteAdaptation(AdaptationNativeTools) != AdaptationAssistedJSON {
		t.Fatal("tools demote to json")
	}
	if DemoteAdaptation(AdaptationAssistedJSON) != AdaptationBaseline {
		t.Fatal("json demote to baseline")
	}
	if DemoteAdaptation(AdaptationBaseline) != AdaptationBaseline {
		t.Fatal("baseline is floor")
	}

	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	profile := BaselineDeclaredProfile("p", "m", MaxOutputDialectLegacy, 2048, now)
	profile.SupportsJSONMode = true
	prev := AdaptationPlan{Level: AdaptationAssistedJSON, ResponseFormat: ResponseFormatJSONObject, Reason: "x"}
	next := PlanAfterDemotion(prev, profile)
	if next.Level != AdaptationBaseline || next.ResponseFormat != ResponseFormatNone {
		t.Fatalf("demoted plan: %+v", next)
	}
	if !ShouldDemote(AdaptationAssistedJSON, AdaptationFailureTransportEnrich) {
		t.Fatal("enrichment failure should demote")
	}
	if ShouldDemote(AdaptationBaseline, AdaptationFailureFormat) {
		t.Fatal("baseline must not demote")
	}
	if ClassifyAdaptationFailure("response_format not supported") != AdaptationFailureTransportEnrich {
		t.Fatal("classify transport")
	}
	if ClassifyAdaptationFailure("invalid json schema") != AdaptationFailureFormat {
		t.Fatal("classify format")
	}

	audit := FormatAdaptationAudit(next)
	if audit == "" || !strings.Contains(audit, "level=") || !strings.Contains(audit, "ctx=") || !strings.Contains(audit, "reason=") {
		t.Fatalf("audit fragment: %q", audit)
	}
}
