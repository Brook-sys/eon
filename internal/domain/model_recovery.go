package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ModelRecoveryStage is one step of the FR-MODEL-004 recovery ladder after the
// deterministic offline normalize path (steps 1–4 in modeltext).
// Stages that contact a provider consume model_calls budget.
type ModelRecoveryStage string

const (
	// RecoveryNormalize is local only (BOM/fence/object extract). No model call.
	RecoveryNormalize ModelRecoveryStage = "NORMALIZE"
	// RecoveryShortCorrection is ladder step 5: one short re-prompt with error + format.
	RecoveryShortCorrection ModelRecoveryStage = "SHORT_CORRECTION"
	// RecoverySimplerFormat is ladder step 6: re-ask with a reduced output contract.
	RecoverySimplerFormat ModelRecoveryStage = "SIMPLER_FORMAT"
	// RecoveryFallbackModel is ladder step 7: one Complete on a configured alternate provider.
	RecoveryFallbackModel ModelRecoveryStage = "FALLBACK_MODEL"
	// RecoveryDefer is ladder step 8: stop contacting the model for this operation.
	RecoveryDefer ModelRecoveryStage = "DEFER"
)

// ModelRecoveryDisposition is the kernel decision after a failed model attempt.
// It is derived purely from budgets and stages already used — never from free text.
type ModelRecoveryDisposition string

const (
	// DispositionShortCorrect: issue at most one short-correction Complete.
	DispositionShortCorrect ModelRecoveryDisposition = "SHORT_CORRECT"
	// DispositionSimplerFormat: recompile/complete with a simpler answer format.
	DispositionSimplerFormat ModelRecoveryDisposition = "SIMPLER_FORMAT"
	// DispositionFallbackModel: one Complete on an alternate ModelProvider (step 7).
	DispositionFallbackModel ModelRecoveryDisposition = "FALLBACK_MODEL"
	// DispositionReplan: REQUEST_REPLAN + RESUME → READY for a later dispatch
	// (only when operation Attempt budget still allows another dispatch).
	DispositionReplan ModelRecoveryDisposition = "REPLAN"
	// DispositionExhaust: terminal EXHAUSTED — no further Complete for this op.
	DispositionExhaust ModelRecoveryDisposition = "EXHAUST"
)

// ModelRecoveryBudget is the persistable, authority-free view of how much
// model contact remains for one Operation under its OperationSpec budget.
// Zero ModelCalls / Attempts mean "no units authorized" (Budget semantics).
type ModelRecoveryBudget struct {
	// MaxModelCalls is OperationSpec.Budget.ModelCalls (hard cap on Complete).
	MaxModelCalls int `json:"max_model_calls"`
	// MaxAttempts is OperationSpec.Budget.Attempts (hard cap on Dispatch cycles).
	MaxAttempts int `json:"max_attempts"`
	// ModelCallsUsed counts Complete invocations already spent this lifetime
	// (including the call that just failed). Callers persist via Operation.Attempt
	// and audit events; this struct itself may be ephemeral within an Execute.
	ModelCallsUsed int `json:"model_calls_used"`
	// OperationAttempt is Operation.Attempt after the current Dispatch.
	OperationAttempt uint32 `json:"operation_attempt"`
	// ShortCorrectionUsed is true once step 5 has been attempted in this Execute.
	ShortCorrectionUsed bool `json:"short_correction_used"`
	// SimplerFormatUsed is true once step 6 has been attempted in this Execute.
	SimplerFormatUsed bool `json:"simpler_format_used"`
	// FallbackModelUsed is true once step 7 has been attempted in this Execute.
	FallbackModelUsed bool `json:"fallback_model_used"`
	// FallbackAvailable is true when the runtime configured an alternate provider.
	// Pure policy never invents providers — the kernel sets this flag.
	FallbackAvailable bool `json:"fallback_available"`
	// AllowReplan permits READY replan when attempt budget remains. When false,
	// budget exhaustion always yields EXHAUST (preferred for always-invalid models).
	AllowReplan bool `json:"allow_replan"`
}

// ModelRecoveryDecision records a pure policy outcome for audit.
type ModelRecoveryDecision struct {
	Disposition ModelRecoveryDisposition `json:"disposition"`
	Stage       ModelRecoveryStage       `json:"stage,omitempty"`
	Reason      string                   `json:"reason"`
	// RemainingModelCalls is MaxModelCalls - ModelCallsUsed after the decision
	// (may be zero). Negative is never returned.
	RemainingModelCalls int `json:"remaining_model_calls"`
}

// NewModelRecoveryBudget derives a budget view from the operation spec and
// current attempt/call counters. allowReplan defaults to true when Attempts > 1.
func NewModelRecoveryBudget(spec OperationSpec, operationAttempt uint32, modelCallsUsed int) ModelRecoveryBudget {
	maxCalls := spec.Budget.ModelCalls
	maxAttempts := spec.Budget.Attempts
	return ModelRecoveryBudget{
		MaxModelCalls:    maxCalls,
		MaxAttempts:      maxAttempts,
		ModelCallsUsed:   modelCallsUsed,
		OperationAttempt: operationAttempt,
		AllowReplan:      maxAttempts > 1 && int(operationAttempt) < maxAttempts,
	}
}

// RemainingModelCalls returns non-negative remaining Complete slots.
func (b ModelRecoveryBudget) RemainingModelCalls() int {
	if b.MaxModelCalls <= 0 {
		return 0
	}
	left := b.MaxModelCalls - b.ModelCallsUsed
	if left < 0 {
		return 0
	}
	return left
}

// DecideNextRecovery chooses the next ladder step after a known non-effect
// validation/provider failure. Pure: no I/O, no clocks.
//
// Order: short correction (step 5) → simpler format (step 6) → fallback model
// (step 7, only if FallbackAvailable) → replan (if attempt budget remains) →
// exhaust (explicit terminal; no call loop).
func DecideNextRecovery(b ModelRecoveryBudget) ModelRecoveryDecision {
	remaining := b.RemainingModelCalls()
	// Step 5: one localized correction when a model call remains.
	if !b.ShortCorrectionUsed && remaining > 0 {
		return ModelRecoveryDecision{
			Disposition:         DispositionShortCorrect,
			Stage:               RecoveryShortCorrection,
			Reason:              "validation_failed_short_correction_available",
			RemainingModelCalls: remaining,
		}
	}
	// Step 6: simpler format if not yet tried and a call remains.
	if !b.SimplerFormatUsed && remaining > 0 {
		return ModelRecoveryDecision{
			Disposition:         DispositionSimplerFormat,
			Stage:               RecoverySimplerFormat,
			Reason:              "validation_failed_simpler_format_available",
			RemainingModelCalls: remaining,
		}
	}
	// Step 7: alternate provider once, when configured and a call remains.
	if b.FallbackAvailable && !b.FallbackModelUsed && remaining > 0 {
		return ModelRecoveryDecision{
			Disposition:         DispositionFallbackModel,
			Stage:               RecoveryFallbackModel,
			Reason:              "validation_failed_fallback_model_available",
			RemainingModelCalls: remaining,
		}
	}
	// Cross-dispatch replan only when Attempts authorize another Dispatch.
	if b.AllowReplan && b.MaxAttempts > 0 && int(b.OperationAttempt) < b.MaxAttempts {
		return ModelRecoveryDecision{
			Disposition:         DispositionReplan,
			Stage:               RecoveryDefer,
			Reason:              "intra_execute_recovery_exhausted_replan_allowed",
			RemainingModelCalls: remaining,
		}
	}
	// Step 8 / FR-MODEL-004 acceptance: explicit terminal, no further Complete.
	return ModelRecoveryDecision{
		Disposition:         DispositionExhaust,
		Stage:               RecoveryDefer,
		Reason:              "model_recovery_budget_exhausted",
		RemainingModelCalls: remaining,
	}
}

// FailureLocusModel is the locus for provider/output recovery failures.
const FailureLocusModel FailureLocus = "MODEL_PROVIDER"

// NewModelValidationFailure builds an immutable FailureRecord for a rejected
// model output (or recovery exhaustion). It does not persist; callers store or
// emit audit events separately. SafeDetail must already be redacted.
func NewModelValidationFailure(
	id FailureID,
	operation Operation,
	attempt uint32,
	code string,
	disposition RetryDisposition,
	safeDetail string,
	policyVersion string,
	now time.Time,
) (FailureRecord, error) {
	if id == "" || operation.ID == "" || strings.TrimSpace(code) == "" || strings.TrimSpace(policyVersion) == "" {
		return FailureRecord{}, errors.New("failure id, operation, code, and policy version are required")
	}
	if now.IsZero() {
		return FailureRecord{}, errors.New("failure occurred_at is required")
	}
	detail := strings.TrimSpace(safeDetail)
	if len(detail) > 512 {
		detail = detail[:512]
	}
	rec := FailureRecord{
		SchemaVersion:    SchemaVersionV1,
		ID:               id,
		Code:             code,
		Class:            FailureValidation,
		Locus:            FailureLocusModel,
		RetryDisposition: disposition,
		EffectState:      EffectNotApplied,
		Scope:            ScopeAttempt,
		MissionRevision:  operation.MissionRevision,
		InquiryID:        operation.InquiryID,
		OperationID:      operation.ID,
		Attempt:          attempt,
		OccurredAt:       now.UTC(),
		SafeDetail:       detail,
		PolicyVersion:    policyVersion,
	}
	if err := rec.Validate(); err != nil {
		return FailureRecord{}, err
	}
	return rec, nil
}

// Validate checks FailureRecord structural integrity (taxonomy baseline).
func (f FailureRecord) Validate() error {
	if f.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported failure schema version %d", f.SchemaVersion)
	}
	if f.ID == "" || f.Code == "" || f.PolicyVersion == "" || f.OccurredAt.IsZero() {
		return errors.New("failure record is missing id, code, policy version, or occurred_at")
	}
	switch f.Class {
	case FailureValidation, FailureAuthority, FailureResource, FailureDependency,
		FailureConflict, FailureIntegrity, FailureSecurity, FailureProgress, FailureInternal:
	default:
		return fmt.Errorf("unknown failure class %q", f.Class)
	}
	switch f.RetryDisposition {
	case NoRetry, RetryNow, RetryAfter, Replan, Reconcile, RequireApproval, Quarantine, PauseMission:
	default:
		return fmt.Errorf("unknown retry disposition %q", f.RetryDisposition)
	}
	switch f.EffectState {
	case EffectNotStarted, EffectNotApplied, EffectApplied, EffectUnknown, EffectPartial:
	default:
		return fmt.Errorf("unknown effect state %q", f.EffectState)
	}
	switch f.Scope {
	case ScopeAttempt, ScopeOperation, ScopeInquiry, ScopeMission, ScopeRuntime:
	default:
		return fmt.Errorf("unknown failure scope %q", f.Scope)
	}
	if f.RetryDisposition == RetryAfter && (f.RetryAt == nil || f.RetryAt.IsZero()) {
		return errors.New("RETRY_AFTER requires retry_at")
	}
	if f.RetryDisposition != RetryAfter && f.RetryAt != nil {
		return errors.New("only RETRY_AFTER may carry retry_at")
	}
	return nil
}

// RetryDispositionForRecovery maps a recovery decision to FailureRecord disposition.
func RetryDispositionForRecovery(d ModelRecoveryDisposition) RetryDisposition {
	switch d {
	case DispositionShortCorrect, DispositionSimplerFormat, DispositionFallbackModel:
		return RetryNow
	case DispositionReplan:
		return Replan
	case DispositionExhaust:
		return NoRetry
	default:
		return NoRetry
	}
}
