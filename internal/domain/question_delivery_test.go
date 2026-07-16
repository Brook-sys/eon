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

func TestQuestionDeliveryExpiredLeaseIsDueAndReclaimable(t *testing.T) {
	base := pendingDelivery()
	leaseUntil := base.CreatedAt.Add(time.Minute)
	leased, err := LeaseQuestionDelivery(base, "worker", base.CreatedAt, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	if leased.Due(leaseUntil.Add(-time.Nanosecond)) || !leased.Due(leaseUntil) {
		t.Fatal("expired lease due boundary is incorrect")
	}
	retry, err := ReclaimExpiredQuestionDelivery(leased, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != QuestionDeliveryRetry || !retry.AvailableAt.Equal(leaseUntil) || retry.LastFailureCode != "LEASE_EXPIRED_RECONCILE" {
		t.Fatalf("reclaimed = %#v", retry)
	}
	if _, err := ReclaimExpiredQuestionDelivery(leased, leaseUntil.Add(-time.Nanosecond)); err == nil {
		t.Fatal("active lease reclaimed")
	}
}
