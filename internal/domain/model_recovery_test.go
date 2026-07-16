package domain

import (
	"testing"
	"time"
)

func TestDecideNextRecoveryLadderAndExhaustion(t *testing.T) {
	t.Parallel()

	// Fresh budget with 2 model calls: prefer short correction.
	b := ModelRecoveryBudget{
		MaxModelCalls: 2, MaxAttempts: 1, ModelCallsUsed: 1, OperationAttempt: 1,
	}
	d := DecideNextRecovery(b)
	if d.Disposition != DispositionShortCorrect || d.Stage != RecoveryShortCorrection {
		t.Fatalf("want short correct, got %+v", d)
	}

	// After short correction, one call left → simpler format.
	b.ShortCorrectionUsed = true
	d = DecideNextRecovery(b)
	if d.Disposition != DispositionSimplerFormat {
		t.Fatalf("want simpler format, got %+v", d)
	}

	// Both recovery stages used, no attempt budget for replan → exhaust.
	b.SimplerFormatUsed = true
	d = DecideNextRecovery(b)
	if d.Disposition != DispositionExhaust {
		t.Fatalf("want exhaust, got %+v", d)
	}

	// Replan when Attempts allow another dispatch.
	b = ModelRecoveryBudget{
		MaxModelCalls: 1, MaxAttempts: 3, ModelCallsUsed: 1, OperationAttempt: 1,
		ShortCorrectionUsed: true, SimplerFormatUsed: true, AllowReplan: true,
	}
	d = DecideNextRecovery(b)
	if d.Disposition != DispositionReplan {
		t.Fatalf("want replan, got %+v", d)
	}

	// Zero model calls authorized → never short-correct; exhaust immediately.
	b = ModelRecoveryBudget{MaxModelCalls: 0, MaxAttempts: 1, ModelCallsUsed: 0, OperationAttempt: 1}
	d = DecideNextRecovery(b)
	if d.Disposition != DispositionExhaust {
		t.Fatalf("zero model calls must exhaust, got %+v", d)
	}
}

func TestNewModelRecoveryBudgetFromSpec(t *testing.T) {
	t.Parallel()
	spec := OperationSpec{
		SchemaVersion: 1, ID: "x@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "i", OutputSchema: "o",
		Budget:          Budget{ModelCalls: 2, Tokens: 1000, Attempts: 2},
		MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"},
		RetryPolicy: "model-recovery@1", FallbackPolicy: "exhaust",
		MaximumAuthority: AuthorityProposeOnly,
	}
	b := NewModelRecoveryBudget(spec, 1, 1)
	if b.MaxModelCalls != 2 || b.MaxAttempts != 2 || !b.AllowReplan {
		t.Fatalf("budget = %+v", b)
	}
	b2 := NewModelRecoveryBudget(spec, 2, 2)
	if b2.AllowReplan {
		t.Fatal("attempt at cap must not allow replan")
	}
}

func TestFailureRecordValidateAndModelValidationFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC)
	op := Operation{
		SchemaVersion: 1, ID: "operation_1", InquiryID: "inquiry_1",
		MissionRevision: "revision_1", SpecID: "extract@1",
		ExpectedOutput: "x", IdempotencyKey: "k",
		State: StateReady, Reevaluation: ReevaluationCondition{Kind: ReevaluateReady},
	}
	rec, err := NewModelValidationFailure(
		"failure_1", op, 1, "MODEL_OUTPUT_INVALID", NoRetry, "strict decode failed", "policy@1", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Validate(); err != nil {
		t.Fatal(err)
	}
	if rec.Class != FailureValidation || rec.EffectState != EffectNotApplied || rec.Locus != FailureLocusModel {
		t.Fatalf("unexpected record: %+v", rec)
	}

	// RETRY_AFTER without instant must fail validate.
	bad := rec
	bad.RetryDisposition = RetryAfter
	if err := bad.Validate(); err == nil {
		t.Fatal("expected RETRY_AFTER without retry_at to fail")
	}
}

func TestRetryDispositionForRecovery(t *testing.T) {
	t.Parallel()
	if RetryDispositionForRecovery(DispositionShortCorrect) != RetryNow {
		t.Fatal("short correct → RETRY_NOW")
	}
	if RetryDispositionForRecovery(DispositionExhaust) != NoRetry {
		t.Fatal("exhaust → NO_RETRY")
	}
	if RetryDispositionForRecovery(DispositionReplan) != Replan {
		t.Fatal("replan → REPLAN")
	}
}
