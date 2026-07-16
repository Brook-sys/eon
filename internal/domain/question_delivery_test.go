package domain

import (
	"testing"
	"time"
)

func pendingDelivery() QuestionDelivery {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	return QuestionDelivery{SchemaVersion: SchemaVersionV1, ID: "delivery_1", QuestionID: "ask_1", QuestionRevision: 1, Channel: "telegram", DestinationRef: "operator_primary", Status: QuestionDeliveryPending, MaxAttempts: 2, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
}

func TestQuestionDeliveryLeaseComplete(t *testing.T) {
	base := pendingDelivery()
	leased, err := LeaseQuestionDelivery(base, "worker_1", base.CreatedAt, base.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if leased.Status != QuestionDeliveryLeased || leased.Attempt != 1 {
		t.Fatalf("leased = %#v", leased)
	}
	delivered, err := CompleteQuestionDelivery(leased, "worker_1", "message_1", base.CreatedAt.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != QuestionDeliveryDelivered || delivered.TransportMessageID != "message_1" || !delivered.Status.Terminal() {
		t.Fatalf("delivered = %#v", delivered)
	}
}

func TestPermanentlyFailQuestionDeliveryPreservesAttemptPolicy(t *testing.T) {
	base := pendingDelivery()
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, base.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	dead, err := PermanentlyFailQuestionDelivery(leased, "worker", "UNAUTHORIZED", base.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if dead.Status != QuestionDeliveryDead || dead.MaxAttempts != base.MaxAttempts || dead.Attempt != 1 || dead.LastFailureCode != "UNAUTHORIZED" {
		t.Fatalf("dead delivery = %#v", dead)
	}
}

func TestQuestionDeliveryRetryAndDeadLetter(t *testing.T) {
	base := pendingDelivery()
	leased, err := LeaseQuestionDelivery(base, "worker_1", base.CreatedAt, base.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	retryAt := base.CreatedAt.Add(5 * time.Minute)
	retry, err := FailQuestionDelivery(leased, "worker_1", "TEMPORARY", base.CreatedAt.Add(time.Second), retryAt)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != QuestionDeliveryRetry || !retry.AvailableAt.Equal(retryAt) || retry.Due(base.CreatedAt) || !retry.Due(retryAt) {
		t.Fatalf("retry = %#v", retry)
	}
	leasedAgain, err := LeaseQuestionDelivery(retry, "worker_2", retryAt, retryAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	dead, err := FailQuestionDelivery(leasedAgain, "worker_2", "PERMANENT", retryAt.Add(time.Second), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if dead.Status != QuestionDeliveryDead || dead.LastFailureCode != "PERMANENT" || !dead.Status.Terminal() {
		t.Fatalf("dead = %#v", dead)
	}
}

func TestQuestionDeliveryRejectsLeaseMismatchAndEarlyRetry(t *testing.T) {
	base := pendingDelivery()
	if _, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt.Add(-time.Second), base.CreatedAt.Add(time.Minute)); err == nil {
		t.Fatal("delivery leased before available")
	}
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, base.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteQuestionDelivery(leased, "other", "message", base.CreatedAt.Add(time.Second)); err == nil {
		t.Fatal("foreign worker completed lease")
	}
	if _, err := FailQuestionDelivery(leased, "worker", "TEMP", base.CreatedAt.Add(time.Second), base.CreatedAt); err == nil {
		t.Fatal("past retry accepted")
	}
}

func TestExpiredLeaseBecomesEffectUnknownAndIsNotAutoleased(t *testing.T) {
	base := pendingDelivery()
	leaseUntil := base.CreatedAt.Add(time.Minute)
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	if leased.Due(leaseUntil.Add(-time.Nanosecond)) || !leased.Due(leaseUntil) {
		t.Fatal("expired lease due boundary is incorrect")
	}
	unknown, err := ReclaimExpiredQuestionDelivery(leased, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Status != QuestionDeliveryEffectUnknown || unknown.LastFailureCode != DeliveryFailureLeaseExpired {
		t.Fatalf("reclaimed = %#v", unknown)
	}
	if unknown.Due(leaseUntil) || unknown.Due(leaseUntil.Add(time.Hour)) {
		t.Fatal("EFFECT_UNKNOWN must never be auto-due for lease")
	}
	if _, err := LeaseQuestionDelivery(unknown, "worker_2", leaseUntil, leaseUntil.Add(time.Minute)); err == nil {
		t.Fatal("EFFECT_UNKNOWN leased without reconciliation")
	}
	if _, err := ReclaimExpiredQuestionDelivery(leased, leaseUntil.Add(-time.Nanosecond)); err == nil {
		t.Fatal("active lease reclaimed")
	}

	// Explicit resolve re-enables retry.
	retry, err := ResolveQuestionDeliveryEffectUnknown(unknown, leaseUntil.Add(time.Second), leaseUntil.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != QuestionDeliveryRetry || !retry.Due(leaseUntil.Add(2*time.Second)) {
		t.Fatalf("resolved retry = %#v", retry)
	}
}

func TestEffectUnknownCanCompleteAfterReconcile(t *testing.T) {
	base := pendingDelivery()
	leaseUntil := base.CreatedAt.Add(time.Minute)
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := MarkQuestionDeliveryEffectUnknown(leased, leaseUntil, DeliveryFailureLeaseExpired)
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := CompleteQuestionDeliveryAfterReconcile(unknown, "msg_recovered", leaseUntil.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Status != QuestionDeliveryDelivered || delivered.TransportMessageID != "msg_recovered" || delivered.LastFailureCode != "" {
		t.Fatalf("reconciled deliver = %#v", delivered)
	}
}

func TestAmbiguousTransportAfterSendParksEffectUnknown(t *testing.T) {
	base := pendingDelivery()
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, base.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := MarkAmbiguousTransportAfterSend(leased, "worker", base.CreatedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Status != QuestionDeliveryEffectUnknown || unknown.LastFailureCode != DeliveryFailureAmbiguousTransport {
		t.Fatalf("ambiguous = %#v", unknown)
	}
	if _, err := MarkAmbiguousTransportAfterSend(leased, "other", base.CreatedAt.Add(time.Second)); err == nil {
		t.Fatal("foreign owner marked ambiguous")
	}
}

func TestResolveEffectUnknownExhaustsToDead(t *testing.T) {
	base := pendingDelivery()
	base.MaxAttempts = 1
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, base.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := MarkQuestionDeliveryEffectUnknown(leased, base.CreatedAt.Add(time.Minute), DeliveryFailureLeaseExpired)
	if err != nil {
		t.Fatal(err)
	}
	dead, err := ResolveQuestionDeliveryEffectUnknown(unknown, base.CreatedAt.Add(time.Minute+time.Second), base.CreatedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if dead.Status != QuestionDeliveryDead {
		t.Fatalf("expected dead after exhausted resolve, got %#v", dead)
	}
}
