package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func dispatchFixture(now time.Time, requestID string, availableAt time.Time) domain.SubagentDispatch {
	return domain.SubagentDispatch{SchemaVersion: 1, RequestID: domain.SubagentDispatchRequestID(requestID), SessionID: "session-1", Attempt: 0, PeerID: "peer-1", Status: domain.SubagentDispatchPending, MaxSendAttempts: 3, AvailableAt: availableAt, CreatedAt: now, UpdatedAt: now}
}

func seedDispatchRecord(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "session-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "do work", TransportPeerID: "peer-1", Attempt: 0, MaxAttempts: 3}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.CreateSubagentRecord(record) }); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentDispatchStorageConflictDueOrderAndCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	seedDispatchRecord(t, store, now)
	later := dispatchFixture(now, "request-b", now.Add(time.Minute))
	earlier := dispatchFixture(now, "request-a", now)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentDispatch(later); err != nil {
			return err
		}
		return tx.CreateSubagentDispatch(earlier)
	}); err == nil || !errors.Is(err, port.ErrConflict) {
		t.Fatalf("duplicate generation error = %v, want conflict", err)
	}
	// Rollback is atomic, so create distinct generations for ordering.
	later.Attempt, later.SessionID = 1, "session-2"
	earlier.Attempt, earlier.SessionID = 0, "session-1"
	record2 := domain.SubagentRecord{SchemaVersion: 1, ID: "session-2", TaskID: "task-2", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "do work", TransportPeerID: "peer-1", Attempt: 1, MaxAttempts: 3}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record2); err != nil {
			return err
		}
		if err := tx.CreateSubagentDispatch(later); err != nil {
			return err
		}
		return tx.CreateSubagentDispatch(earlier)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		due, err := r.DueSubagentDispatches(now.Add(time.Minute), 10)
		if err != nil {
			return err
		}
		if len(due) != 2 || due[0].RequestID != "request-a" || due[1].RequestID != "request-b" {
			t.Fatalf("due order = %+v", due)
		}
		byGeneration, err := r.SubagentDispatchByGeneration("session-2", 1)
		if err != nil || byGeneration.RequestID != "request-b" {
			t.Fatalf("generation lookup = %+v, %v", byGeneration, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	leased, err := domain.LeaseSubagentDispatch(earlier, "worker", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(leased, earlier.Status, earlier.SendAttempt)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(leased, earlier.Status, earlier.SendAttempt)
	}); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("stale save error = %v", err)
	}

	checkpoint, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := memory.NewFromBinary(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentDispatch("request-a")
		if err != nil {
			return err
		}
		if got.Status != domain.SubagentDispatchLeased || got.SendAttempt != 1 {
			t.Fatalf("checkpoint dispatch = %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
