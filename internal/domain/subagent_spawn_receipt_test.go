package domain

import (
	"strings"
	"testing"
	"time"
)

func spawnReceiptFixture(now time.Time) SubagentSpawnReceipt {
	return SubagentSpawnReceipt{
		SchemaVersion:     SchemaVersionV1,
		CallerPeerID:      "peer-a",
		RequestID:         "request-a",
		SourceSessionID:   "source-a",
		Attempt:           1,
		Task:              "do work",
		ContextMode:       "isolated",
		ReceiverSessionID: "receiver-a",
		RecordedAt:        now,
		Status:            SubagentSpawnReceiptPending,
		UpdatedAt:         now,
	}
}

func TestSubagentSpawnReceiptQueueTransitions(t *testing.T) {
	now := time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)
	pending := spawnReceiptFixture(now)
	if err := pending.Validate(); err != nil || !pending.Due(now) {
		t.Fatalf("pending receipt invalid or not due: %v", err)
	}
	leased, err := LeaseSubagentSpawnReceipt(pending, "worker-a", now.Add(time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if leased.Due(now.Add(30*time.Second)) || !leased.Due(now.Add(time.Minute)) {
		t.Fatal("leased receipt due semantics are incorrect")
	}
	complete, err := CompleteSubagentSpawnReceipt(leased, "worker-a", "result", now.Add(30*time.Second))
	if err != nil || complete.Status != SubagentSpawnReceiptComplete || complete.Result != "result" || complete.Due(now.Add(time.Hour)) {
		t.Fatalf("complete = %+v, %v", complete, err)
	}
	inFlight, err := BeginSubagentSpawnReceiptStatusDelivery(complete, now.Add(31*time.Second))
	if err != nil || inFlight.StatusDelivery != SubagentStatusDeliveryInFlight {
		t.Fatalf("in flight = %+v, %v", inFlight, err)
	}
	delivered, err := MarkSubagentSpawnReceiptStatusDelivered(inFlight, now.Add(32*time.Second))
	if err != nil || delivered.StatusDelivery != SubagentStatusDeliveryDelivered || delivered.Result != complete.Result {
		t.Fatalf("delivered = %+v, %v", delivered, err)
	}
	if _, err := MarkSubagentSpawnReceiptStatusDelivered(delivered, now.Add(33*time.Second)); err == nil {
		t.Fatal("accepted duplicate status acknowledgement")
	}

	leased, err = LeaseSubagentSpawnReceipt(pending, "worker-a", now.Add(time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	failed, err := FailSubagentSpawnReceipt(leased, "worker-a", "failed", now.Add(30*time.Second))
	if err != nil || failed.Status != SubagentSpawnReceiptFailed || failed.Failure != "failed" {
		t.Fatalf("failed = %+v, %v", failed, err)
	}
}

func TestSubagentSpawnReceiptExpiredLeaseRecoveryAndBounds(t *testing.T) {
	now := time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)
	leased, err := LeaseSubagentSpawnReceipt(spawnReceiptFixture(now), "worker-a", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverExpiredSubagentSpawnReceipt(leased, now.Add(59*time.Second)); err == nil {
		t.Fatal("recovered unexpired lease")
	}
	recovered, err := RecoverExpiredSubagentSpawnReceipt(leased, now.Add(time.Minute))
	if err != nil || recovered.Status != SubagentSpawnReceiptPending || recovered.LeaseOwner != "" {
		t.Fatalf("recovered = %+v, %v", recovered, err)
	}
	leased, _ = LeaseSubagentSpawnReceipt(recovered, "worker-b", now.Add(time.Minute), now.Add(2*time.Minute))
	if _, err := CompleteSubagentSpawnReceipt(leased, "worker-b", strings.Repeat("x", MaxSubagentSpawnResultBytes+1), now.Add(90*time.Second)); err == nil {
		t.Fatal("accepted oversized result")
	}
	if _, err := FailSubagentSpawnReceipt(leased, "worker-b", strings.Repeat("x", MaxSubagentSpawnFailureBytes+1), now.Add(90*time.Second)); err == nil {
		t.Fatal("accepted oversized failure")
	}
}

func TestSubagentSpawnReceiptLegacyQueueDefaults(t *testing.T) {
	now := time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)
	legacy := spawnReceiptFixture(now)
	legacy.Status, legacy.UpdatedAt = "", time.Time{}
	if err := legacy.Validate(); err != nil || !legacy.Due(now) {
		t.Fatalf("legacy receipt invalid or not due: %v", err)
	}
	leased, err := LeaseSubagentSpawnReceipt(legacy, "worker", now, now.Add(time.Minute))
	if err != nil || leased.Status != SubagentSpawnReceiptLeased || !leased.UpdatedAt.Equal(now) {
		t.Fatalf("leased legacy = %+v, %v", leased, err)
	}
}
