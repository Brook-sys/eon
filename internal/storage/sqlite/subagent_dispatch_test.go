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

func TestSubagentDispatchSurvivesSQLiteRestart(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "session-1", TaskID: "task-1", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "work", TransportPeerID: "peer-1", Attempt: 0, MaxAttempts: 3}
	dispatch := domain.SubagentDispatch{SchemaVersion: 1, RequestID: "request-1", SessionID: record.ID, Attempt: 0, PeerID: record.TransportPeerID, Status: domain.SubagentDispatchPending, MaxSendAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentDispatch(dispatch)
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
		got, err := r.SubagentDispatchByGeneration(record.ID, 0)
		if err != nil {
			return err
		}
		if got.RequestID != dispatch.RequestID || got.Status != domain.SubagentDispatchPending {
			t.Fatalf("restarted dispatch = %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
