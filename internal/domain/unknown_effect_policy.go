package domain

import (
	"errors"
	"strings"
	"time"
)

// UnknownEffectPolicySchemaVersion is the contract version for the durable
// representation of a kernel-owned unknown-effect decision.
const UnknownEffectPolicySchemaVersion = 1

// UnknownEffectEvidence is the validated, kernel-owned evidence that an
// unknown-effect disposition consumes. It is never derived from free model
// text; it is extracted by the kernel from durable receipts or status records
// that the transport layer has already authenticated and persisted.
//
// All fields are redacted/sanitized before persistence — no model completion
// text, no raw provider payload, no secret material.
type UnknownEffectEvidence struct {
	// SchemaVersion must match UnknownEffectPolicySchemaVersion.
	SchemaVersion int `json:"schema_version"`
	// Source identifies the durable artifact that the kernel extracted this
	// evidence from (e.g. "subagent_dispatch:dispatch_001", "model_completion:op_x:1:3").
	// It is a non-authoritative label for audit; the kernel validated the
	// underlying record before constructing this struct.
	Source string `json:"source"`
	// EffectState is the authenticated effect classification from the durable
	// record. Must be EffectUnknown or EffectPartial — never EffectApplied.
	EffectState EffectState `json:"effect_state"`
	// DeliveryReceipt is true when a delivery receipt exists confirming the
	// request reached the remote endpoint but the response is ambiguous.
	DeliveryReceipt bool `json:"delivery_receipt"`
	// RemoteStatus is true when a remote status record was received but did
	// not disambiguate the effect (e.g. timeout, partial write).
	RemoteStatus bool `json:"remote_status"`
	// ReconcileAttempts counts how many bounded reconciliation attempts the
	// kernel has already performed on this effect. Zero means no reconciliation
	// has been attempted yet.
	ReconcileAttempts int `json:"reconcile_attempts"`
	// LastReconcileAt is the recorded wall-clock time of the most recent
	// reconciliation attempt, or zero if none has occurred. This pure value
	// object validates consistency with ReconcileAttempts; future-time checks
	// belong at the clock-aware ingestion boundary.
	LastReconcileAt time.Time `json:"last_reconcile_at,omitempty"`
	// ModelSuggestedDecision records what a model suggested (if any), purely for
	// audit. The kernel never consults this field when deciding the disposition.
	// It is preserved to prove that the kernel-overrode model authority.
	ModelSuggestedDecision string `json:"model_suggested_decision,omitempty"`
}

// Validate checks structural integrity without consulting any external state.
func (e UnknownEffectEvidence) Validate() error {
	if e.SchemaVersion != UnknownEffectPolicySchemaVersion {
		return errors.New("unknown effect evidence: unsupported schema version")
	}
	if strings.TrimSpace(e.Source) == "" {
		return errors.New("unknown effect evidence: source is required")
	}
	if e.EffectState != EffectUnknown && e.EffectState != EffectPartial {
		return errors.New("unknown effect evidence: effect_state must be UNKNOWN or PARTIAL")
	}
	if !e.DeliveryReceipt && !e.RemoteStatus {
		return errors.New("unknown effect evidence: at least one of delivery_receipt or remote_status is required")
	}
	if e.ReconcileAttempts < 0 {
		return errors.New("unknown effect evidence: reconcile_attempts must not be negative")
	}
	if !e.LastReconcileAt.IsZero() && e.ReconcileAttempts == 0 {
		return errors.New("unknown effect evidence: last_reconcile_at set but reconcile_attempts is zero")
	}
	return nil
}

// UnknownEffectDisposition is the kernel decision for an unknown effect.
// It is either a bounded kernel-owned reconciliation step or terminal DEFER;
// the kernel never delegates RETRY authority to model output. This is the
// safety-critical invariant proven by the Phase 273 cross-model campaign and
// codified here.
type UnknownEffectDisposition string

const (
	// UnknownEffectDefer is the terminal disposition: stop contacting the
	// model for this effect; the kernel owns the retry/deferral decision.
	UnknownEffectDefer UnknownEffectDisposition = "DEFER"
	// UnknownEffectReconcile is assigned when the kernel still has
	// reconciliation budget (ReconcileAttempts < max) and will attempt a
	// bounded, deterministic reconciliation without model assistance.
	UnknownEffectReconcile UnknownEffectDisposition = "RECONCILE"
)

// UnknownEffectDecision is the kernel-owned, deterministic output of the
// unknown-effect policy. It is derived purely from evidence and configured
// limits — never from model text.
type UnknownEffectDecision struct {
	SchemaVersion   int                      `json:"schema_version"`
	Disposition     UnknownEffectDisposition `json:"disposition"`
	Reason          string                   `json:"reason"`
	ReconcileBudget int                      `json:"reconcile_budget_remaining"`
	EffectState     EffectState              `json:"effect_state"`
	ModelOverridden bool                     `json:"model_overridden"`
}

// Validate checks that a persisted decision satisfies the versioned contract.
func (d UnknownEffectDecision) Validate() error {
	if d.SchemaVersion != UnknownEffectPolicySchemaVersion {
		return errors.New("unknown effect decision: unsupported schema version")
	}
	if d.Disposition != UnknownEffectDefer && d.Disposition != UnknownEffectReconcile {
		return errors.New("unknown effect decision: invalid disposition")
	}
	if strings.TrimSpace(d.Reason) == "" {
		return errors.New("unknown effect decision: reason is required")
	}
	if d.ReconcileBudget < 0 {
		return errors.New("unknown effect decision: reconcile budget must not be negative")
	}
	if d.EffectState != EffectUnknown && d.EffectState != EffectPartial {
		return errors.New("unknown effect decision: effect_state must be UNKNOWN or PARTIAL")
	}
	if d.Disposition == UnknownEffectDefer && d.ReconcileBudget != 0 {
		return errors.New("unknown effect decision: DEFER must have zero reconcile budget")
	}
	if d.Disposition == UnknownEffectReconcile && d.ReconcileBudget == 0 {
		return errors.New("unknown effect decision: RECONCILE requires positive reconcile budget")
	}
	return nil
}

// UnknownEffectPolicyConfig is the immutable kernel configuration for the
// unknown-effect policy. It sets the maximum reconciliation attempts before
// a terminal DEFER. The kernel never consults model output for this decision.
type UnknownEffectPolicyConfig struct {
	// MaxReconcileAttempts is the hard ceiling on bounded reconciliation
	// attempts before the policy returns terminal DEFER. Must be >= 0.
	// Zero means immediate DEFER on first observation.
	MaxReconcileAttempts int `json:"max_reconcile_attempts"`
}

// Validate checks the configuration.
func (c UnknownEffectPolicyConfig) Validate() error {
	if c.MaxReconcileAttempts < 0 {
		return errors.New("unknown effect policy: max_reconcile_attempts must not be negative")
	}
	return nil
}

// DefaultUnknownEffectPolicyConfig returns a conservative default: one
// bounded reconciliation attempt, then terminal DEFER.
func DefaultUnknownEffectPolicyConfig() UnknownEffectPolicyConfig {
	return UnknownEffectPolicyConfig{MaxReconcileAttempts: 1}
}

// DecideUnknownEffect is the pure, deterministic kernel policy for unknown
// effects. It consumes only validated evidence and configuration — never model
// text. The decision is always one of:
//
//   - RECONCILE: the kernel still has reconciliation budget and will attempt
//     a bounded, deterministic reconciliation without model assistance.
//   - DEFER: terminal — stop contacting the model for this effect.
//
// The function sets ModelOverridden to true when the evidence records a
// model suggestion (ModelSuggestedDecision), proving the kernel independently
// overrode model authority rather than blindly following it.
//
// This function is pure: no I/O, no clocks, no side effects.
func DecideUnknownEffect(evidence UnknownEffectEvidence, config UnknownEffectPolicyConfig) (UnknownEffectDecision, error) {
	if err := evidence.Validate(); err != nil {
		return UnknownEffectDecision{}, err
	}
	if err := config.Validate(); err != nil {
		return UnknownEffectDecision{}, err
	}

	overridden := strings.TrimSpace(evidence.ModelSuggestedDecision) != ""

	remaining := config.MaxReconcileAttempts - evidence.ReconcileAttempts
	if remaining < 0 {
		remaining = 0
	}

	if remaining > 0 {
		return UnknownEffectDecision{
			SchemaVersion:   UnknownEffectPolicySchemaVersion,
			Disposition:     UnknownEffectReconcile,
			Reason:          "kernel_owned_reconciliation_budget_remaining",
			ReconcileBudget: remaining,
			EffectState:     evidence.EffectState,
			ModelOverridden: overridden,
		}, nil
	}

	return UnknownEffectDecision{
		SchemaVersion:   UnknownEffectPolicySchemaVersion,
		Disposition:     UnknownEffectDefer,
		Reason:          "kernel_owned_reconcile_budget_exhausted",
		ReconcileBudget: 0,
		EffectState:     evidence.EffectState,
		ModelOverridden: overridden,
	}, nil
}
