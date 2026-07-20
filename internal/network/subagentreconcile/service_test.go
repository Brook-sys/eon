package subagentreconcile

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestServiceMatchesAuthenticatedSpawnReceiptAndFailsClosed(t *testing.T) {
	store := memory.New()
	now := time.Unix(100, 0).UTC()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "receiver-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", MaxAttempts: 2}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", RequestID: "req-1", SourceSessionID: "source-1", Attempt: 1, Task: "work", ContextMode: "isolated", ReceiverSessionID: record.ID, RecordedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	service, _ := NewService(store)
	digest, _ := domain.SubagentSpawnRequestDigest(domain.SubagentSpawnRequest{RequestID: receipt.RequestID, SessionID: receipt.SourceSessionID, Attempt: receipt.Attempt, Task: receipt.Task, ContextMode: receipt.ContextMode})
	payload, _ := domain.EncodeSubagentReconcileRequest(domain.SubagentReconcileRequest{Kind: domain.SubagentReconcileSpawn, DeliveryID: receipt.RequestID, SessionID: receipt.SourceSessionID, Attempt: receipt.Attempt, Digest: digest})
	encoded, err := service.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	response, _ := domain.DecodeSubagentReconcileResponse(encoded)
	if response.State != domain.SubagentReconcileFound || response.ReceiverSessionID != record.ID {
		t.Fatalf("response=%+v", response)
	}
	encoded, err = service.Handle(context.Background(), "peer-b", payload)
	if err != nil {
		t.Fatal(err)
	}
	response, _ = domain.DecodeSubagentReconcileResponse(encoded)
	if response.State != domain.SubagentReconcileNotFound {
		t.Fatalf("cross-peer response=%+v", response)
	}
}
