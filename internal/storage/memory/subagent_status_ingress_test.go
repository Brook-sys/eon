package memory

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestSubagentStatusIngressCheckpoint(t *testing.T) {
	store := New()
	now := time.Unix(100, 0).UTC()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "session-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", TransportPeerID: "peer-a", MaxAttempts: 2, Deadline: now.Add(time.Minute)}
	receipt := domain.SubagentStatusIngressReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", DeliveryID: "delivery-1", SessionID: record.ID, State: "COMPLETE", Result: "done", Status: domain.SubagentStatusIngressPending, RecordedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	blob, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewFromBinary(blob)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-1")
		if err != nil {
			return err
		}
		if !got.Matches(receipt) {
			t.Fatalf("got=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
