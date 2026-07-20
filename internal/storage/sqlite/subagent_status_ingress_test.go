package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestSubagentStatusIngressSurvivesSQLiteRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	now := time.Unix(100, 0).UTC()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "session-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", TransportPeerID: "peer-a", MaxAttempts: 2, Deadline: now.Add(time.Minute)}
	receipt := domain.SubagentStatusIngressReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", DeliveryID: "delivery-1", SessionID: record.ID, State: "FAILED", Failure: "boom", Status: domain.SubagentStatusIngressPending, RecordedAt: now}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.View(context.Background(), func(r port.Reader) error {
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
