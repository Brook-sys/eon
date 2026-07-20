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
