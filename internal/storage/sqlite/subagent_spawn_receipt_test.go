package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	storage "motor-autonomo/internal/storage/sqlite"
)

func TestSubagentSpawnReceiptSurvivesSQLiteRestart(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 40, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "receiver-1", TaskID: "scoped-task", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", ContextMode: "isolated", Attempt: 0, MaxAttempts: 3}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", RequestID: "request-1", SourceSessionID: "source-1", Attempt: 2, Task: "work", ContextMode: "isolated", ReceiverSessionID: record.ID, RecordedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentSpawnReceipt("peer-a", "request-1")
		if err != nil {
			return err
		}
		if got != receipt || got.Acknowledgement().ReceiverSessionID != record.ID {
			t.Fatalf("restarted receipt = %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalSubagentSpawnReceiptStatusDeliverySurvivesSQLiteRestart(t *testing.T) {
	now := time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "receiver-terminal", TaskID: "scoped-terminal", MissionID: "mission-1", State: domain.SubagentStateComplete, StartedAt: now, UpdatedAt: now.Add(time.Second), Task: "work", ContextMode: "isolated", Result: "answer", MaxAttempts: 3}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-a", RequestID: "request-terminal", SourceSessionID: "source-terminal", Attempt: 3, Task: "work", ContextMode: "isolated", ReceiverSessionID: record.ID, RecordedAt: now, Status: domain.SubagentSpawnReceiptComplete, UpdatedAt: now.Add(time.Second), Result: "answer"}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.View(context.Background(), func(r port.Reader) error {
		pending, err := r.TerminalUndeliveredSubagentSpawnReceipts(2)
		if err != nil {
			return err
		}
		if len(pending) != 1 || pending[0] != receipt {
			t.Fatalf("restarted terminal receipts = %+v", pending)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		current, err := tx.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if err != nil {
			return err
		}
		inFlight, err := domain.BeginSubagentSpawnReceiptStatusDelivery(current, now.Add(2*time.Second))
		if err != nil {
			return err
		}
		if err := tx.SaveSubagentSpawnReceipt(inFlight, current.Status, current.UpdatedAt); err != nil {
			return err
		}
		next, err := domain.MarkSubagentSpawnReceiptStatusDelivered(inFlight, now.Add(3*time.Second))
		if err != nil {
			return err
		}
		return tx.SaveSubagentSpawnReceipt(next, inFlight.Status, inFlight.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		pending, err := r.TerminalUndeliveredSubagentSpawnReceipts(2)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			t.Fatalf("delivered terminal receipts = %+v", pending)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
