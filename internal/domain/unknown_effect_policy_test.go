package domain

import (
	"strings"
	"testing"
	"time"
)

func TestUnknownEffectPolicy_AlwaysDeferWhenBudgetExhausted(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:     UnknownEffectPolicySchemaVersion,
		Source:            "subagent_dispatch:dispatch_001",
		EffectState:       EffectUnknown,
		DeliveryReceipt:   true,
		ReconcileAttempts: 1, // budget exhausted (default max=1)
	}
	config := DefaultUnknownEffectPolicyConfig() // max=1

	dec, err := DecideUnknownEffect(evidence, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Disposition != UnknownEffectDefer {
		t.Fatalf("expected DEFER, got %s", dec.Disposition)
	}
	if dec.ReconcileBudget != 0 {
		t.Fatalf("expected zero reconcile budget, got %d", dec.ReconcileBudget)
	}
}

func TestUnknownEffectPolicy_ReconcileWhenBudgetRemains(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:     UnknownEffectPolicySchemaVersion,
		Source:            "model_completion:op_x:1:3",
		EffectState:       EffectPartial,
		RemoteStatus:      true,
		ReconcileAttempts: 0, // budget remaining
	}
	config := DefaultUnknownEffectPolicyConfig() // max=1

	dec, err := DecideUnknownEffect(evidence, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Disposition != UnknownEffectReconcile {
		t.Fatalf("expected RECONCILE, got %s", dec.Disposition)
	}
	if dec.ReconcileBudget != 1 {
		t.Fatalf("expected reconcile budget 1, got %d", dec.ReconcileBudget)
	}
}

func TestUnknownEffectPolicy_ModelOverriddenFlag(t *testing.T) {
	// Model suggested "RETRY" — kernel must override to DEFER or RECONCILE
	// without consulting the suggestion.
	evidence := UnknownEffectEvidence{
		SchemaVersion:          UnknownEffectPolicySchemaVersion,
		Source:                 "subagent_dispatch:dispatch_002",
		EffectState:            EffectUnknown,
		DeliveryReceipt:        true,
		ReconcileAttempts:      1, // exhausted
		ModelSuggestedDecision: "RETRY",
	}
	config := DefaultUnknownEffectPolicyConfig()

	dec, err := DecideUnknownEffect(evidence, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dec.ModelOverridden {
		t.Fatal("expected ModelOverridden=true when model suggested a decision")
	}
	if dec.Disposition != UnknownEffectDefer {
		t.Fatalf("expected DEFER regardless of model suggestion, got %s", dec.Disposition)
	}
}

func TestUnknownEffectPolicy_NoModelSuggestion_NoOverrideFlag(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:     UnknownEffectPolicySchemaVersion,
		Source:            "subagent_dispatch:dispatch_003",
		EffectState:       EffectUnknown,
		RemoteStatus:      true,
		ReconcileAttempts: 1,
	}
	config := DefaultUnknownEffectPolicyConfig()

	dec, err := DecideUnknownEffect(evidence, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.ModelOverridden {
		t.Fatal("expected ModelOverridden=false when no model suggestion present")
	}
}

func TestUnknownEffectPolicy_NeverReturnsRetry(t *testing.T) {
	// Critical invariant: the policy must NEVER return a RETRY disposition.
	// Only DEFER or RECONCILE are valid outputs.
	cases := []struct {
		name     string
		evidence UnknownEffectEvidence
		config   UnknownEffectPolicyConfig
	}{
		{
			name: "unknown_effect_zero_reconcile",
			evidence: UnknownEffectEvidence{
				SchemaVersion: UnknownEffectPolicySchemaVersion,
				Source:        "s1", EffectState: EffectUnknown,
				DeliveryReceipt: true, ReconcileAttempts: 0,
			},
			config: UnknownEffectPolicyConfig{MaxReconcileAttempts: 0},
		},
		{
			name: "partial_effect_budget_remaining",
			evidence: UnknownEffectEvidence{
				SchemaVersion: UnknownEffectPolicySchemaVersion,
				Source:        "s2", EffectState: EffectPartial,
				RemoteStatus: true, ReconcileAttempts: 0,
			},
			config: UnknownEffectPolicyConfig{MaxReconcileAttempts: 3},
		},
		{
			name: "unknown_effect_mid_reconcile",
			evidence: UnknownEffectEvidence{
				SchemaVersion: UnknownEffectPolicySchemaVersion,
				Source:        "s3", EffectState: EffectUnknown,
				DeliveryReceipt: true, RemoteStatus: true,
				ReconcileAttempts: 2,
			},
			config: UnknownEffectPolicyConfig{MaxReconcileAttempts: 5},
		},
		{
			name: "unknown_effect_exhausted",
			evidence: UnknownEffectEvidence{
				SchemaVersion: UnknownEffectPolicySchemaVersion,
				Source:        "s4", EffectState: EffectUnknown,
				DeliveryReceipt: true, ReconcileAttempts: 5,
			},
			config: UnknownEffectPolicyConfig{MaxReconcileAttempts: 5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := DecideUnknownEffect(tc.evidence, tc.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if dec.Disposition != UnknownEffectDefer && dec.Disposition != UnknownEffectReconcile {
				t.Fatalf("disposition must be DEFER or RECONCILE, got %s", dec.Disposition)
			}
			if strings.Contains(string(dec.Disposition), "RETRY") {
				t.Fatalf("policy must never return RETRY, got %s", dec.Disposition)
			}
		})
	}
}

func TestUnknownEffectPolicy_RejectsAppliedEffect(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:   UnknownEffectPolicySchemaVersion,
		Source:          "s5",
		EffectState:     EffectApplied, // invalid for unknown-effect policy
		DeliveryReceipt: true,
	}
	config := DefaultUnknownEffectPolicyConfig()

	if _, err := DecideUnknownEffect(evidence, config); err == nil {
		t.Fatal("expected error for EffectApplied, got nil")
	}
}

func TestUnknownEffectPolicy_RejectsNoEvidenceSource(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:   UnknownEffectPolicySchemaVersion,
		Source:          "", // missing
		EffectState:     EffectUnknown,
		DeliveryReceipt: true,
	}
	config := DefaultUnknownEffectPolicyConfig()

	if _, err := DecideUnknownEffect(evidence, config); err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestUnknownEffectPolicy_RejectsNoEvidenceSignal(t *testing.T) {
	// Must have at least one of delivery_receipt or remote_status
	evidence := UnknownEffectEvidence{
		SchemaVersion: UnknownEffectPolicySchemaVersion,
		Source:        "s6",
		EffectState:   EffectUnknown,
		// no signals
	}
	config := DefaultUnknownEffectPolicyConfig()

	if _, err := DecideUnknownEffect(evidence, config); err == nil {
		t.Fatal("expected error for no evidence signal, got nil")
	}
}

func TestUnknownEffectPolicy_RejectsInconsistentReconcileFields(t *testing.T) {
	// LastReconcileAt set but ReconcileAttempts == 0 is inconsistent
	evidence := UnknownEffectEvidence{
		SchemaVersion:     UnknownEffectPolicySchemaVersion,
		Source:            "s7",
		EffectState:       EffectUnknown,
		DeliveryReceipt:   true,
		ReconcileAttempts: 0,
		LastReconcileAt:   time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
	}
	config := DefaultUnknownEffectPolicyConfig()

	if _, err := DecideUnknownEffect(evidence, config); err == nil {
		t.Fatal("expected error for inconsistent reconcile fields, got nil")
	}
}

func TestUnknownEffectPolicy_RejectsBadSchemaVersion(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:   999,
		Source:          "s8",
		EffectState:     EffectUnknown,
		DeliveryReceipt: true,
	}
	config := DefaultUnknownEffectPolicyConfig()

	if _, err := DecideUnknownEffect(evidence, config); err == nil {
		t.Fatal("expected error for bad schema version, got nil")
	}
}

func TestUnknownEffectPolicy_RejectsNegativeMaxReconcile(t *testing.T) {
	evidence := UnknownEffectEvidence{
		SchemaVersion:   UnknownEffectPolicySchemaVersion,
		Source:          "s9",
		EffectState:     EffectUnknown,
		DeliveryReceipt: true,
	}
	config := UnknownEffectPolicyConfig{MaxReconcileAttempts: -1}

	if _, err := DecideUnknownEffect(evidence, config); err == nil {
		t.Fatal("expected error for negative max reconcile, got nil")
	}
}

func TestUnknownEffectPolicy_ZeroMaxImmediateDefer(t *testing.T) {
	// When MaxReconcileAttempts=0, even a fresh evidence must DEFER immediately.
	evidence := UnknownEffectEvidence{
		SchemaVersion:     UnknownEffectPolicySchemaVersion,
		Source:            "s10",
		EffectState:       EffectUnknown,
		DeliveryReceipt:   true,
		ReconcileAttempts: 0,
	}
	config := UnknownEffectPolicyConfig{MaxReconcileAttempts: 0}

	dec, err := DecideUnknownEffect(evidence, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Disposition != UnknownEffectDefer {
		t.Fatalf("expected immediate DEFER with max=0, got %s", dec.Disposition)
	}
}

func TestUnknownEffectPolicy_PureDeterministic(t *testing.T) {
	// Same inputs must always produce same output (pure function, no I/O).
	evidence := UnknownEffectEvidence{
		SchemaVersion:     UnknownEffectPolicySchemaVersion,
		Source:            "s11",
		EffectState:       EffectPartial,
		RemoteStatus:      true,
		ReconcileAttempts: 1,
	}
	config := UnknownEffectPolicyConfig{MaxReconcileAttempts: 3}

	dec1, err1 := DecideUnknownEffect(evidence, config)
	dec2, err2 := DecideUnknownEffect(evidence, config)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v %v", err1, err2)
	}
	if dec1 != dec2 {
		t.Fatalf("non-deterministic: %#v != %#v", dec1, dec2)
	}
}
