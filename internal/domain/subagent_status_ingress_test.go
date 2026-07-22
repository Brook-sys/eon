package domain

import (
	"errors"
	"testing"
	"time"
)

func TestSubagentStatusIngressTransitions(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	receipt := SubagentStatusIngressReceipt{SchemaVersion: SchemaVersionV1, CallerPeerID: "peer-a", DeliveryID: "delivery-1", SessionID: "session-1", State: "COMPLETE", Result: "done", Status: SubagentStatusIngressPending, RecordedAt: now}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	applied, err := MarkSubagentStatusIngressApplied(receipt, now.Add(time.Second))
	if err != nil || applied.Status != SubagentStatusIngressApplied {
		t.Fatalf("applied=%+v err=%v", applied, err)
	}
	if _, err := MarkSubagentStatusIngressApplied(applied, now.Add(2*time.Second)); !errors.Is(err, ErrInvalidSubagentStatusIngress) {
		t.Fatalf("reapply=%v", err)
	}
	divergent := receipt
	divergent.Result = "other"
	if receipt.Matches(divergent) {
		t.Fatal("divergent immutable payload matched")
	}
}

func TestSubagentStatusIngressAcceptsPayloadFreeRunningHeartbeat(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	receipt := SubagentStatusIngressReceipt{SchemaVersion: SchemaVersionV1, CallerPeerID: "peer-a", DeliveryID: "heartbeat-1", SessionID: "session-1", Attempt: 0, State: "RUNNING", Status: SubagentStatusIngressPending, RecordedAt: now}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt.Result = "not allowed"
	if err := receipt.Validate(); !errors.Is(err, ErrInvalidSubagentStatusIngress) {
		t.Fatalf("running heartbeat with payload error = %v", err)
	}
}

func TestRejectSubagentStatusIngressAttemptMismatch(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	receipt := SubagentStatusIngressReceipt{SchemaVersion: SchemaVersionV1, CallerPeerID: "peer-a", DeliveryID: "delivery-stale", SessionID: "session-1", Attempt: 0, State: "COMPLETE", Result: "late", Status: SubagentStatusIngressPending, RecordedAt: now}
	rejected, err := RejectSubagentStatusIngressAttemptMismatch(receipt, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != SubagentStatusIngressRejected || rejected.RejectionCode != SubagentStatusIngressRejectionAttemptMismatch || rejected.RejectedAt.IsZero() {
		t.Fatalf("rejected=%+v", rejected)
	}
	if _, err := MarkSubagentStatusIngressApplied(rejected, now.Add(2*time.Second)); !errors.Is(err, ErrInvalidSubagentStatusIngress) {
		t.Fatalf("apply rejected err=%v", err)
	}
}

func TestRejectSubagentStatusIngressTerminalConflict(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	receipt := SubagentStatusIngressReceipt{SchemaVersion: SchemaVersionV1, CallerPeerID: "peer-a", DeliveryID: "delivery-conflict", SessionID: "session-1", Attempt: 1, State: "FAILED", Failure: "contradiction", Status: SubagentStatusIngressPending, RecordedAt: now}
	rejected, err := RejectSubagentStatusIngressTerminalConflict(receipt, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != SubagentStatusIngressRejected || rejected.RejectionCode != SubagentStatusIngressRejectionTerminalConflict || rejected.RejectedAt.IsZero() {
		t.Fatalf("rejected=%+v", rejected)
	}
	if !rejected.Matches(receipt) || !rejected.RecordedAt.Equal(receipt.RecordedAt) {
		t.Fatalf("rejection changed immutable evidence: receipt=%+v rejected=%+v", receipt, rejected)
	}
	invalid := rejected
	invalid.RejectionCode = "UNKNOWN"
	if !errors.Is(invalid.Validate(), ErrInvalidSubagentStatusIngress) {
		t.Fatalf("unknown rejection code validated: %+v", invalid)
	}
}
