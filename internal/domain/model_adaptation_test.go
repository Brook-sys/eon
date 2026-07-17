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
