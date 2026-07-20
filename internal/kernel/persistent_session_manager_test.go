package kernel_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestPersistentSessionManagerPersistsSpawnAndKeepsItIdempotent(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := kernel.NewPersistentSessionManager(local, store, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
		MissionID: "mission-1", MaxAttempts: 2, Timeout: 5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := kernel.SubagentSpec{Task: "inspect code", ContextMode: "isolated", Labels: map[string]string{"task_id": "task-1"}}
	first, err := manager.Spawn(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Spawn(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("idempotent spawn ids differ: %q != %q", first, second)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, readErr := r.SubagentRecord(string(first))
		if readErr != nil {
			return readErr
		}
		if record.State != domain.SubagentStatePending || record.TaskID != "task-1" || record.MissionID != "mission-1" || record.MaxAttempts != 2 {
			t.Fatalf("unexpected durable record: %+v", record)
		}
		if want := clock.Now().Add(5 * time.Minute); !record.Deadline.Equal(want) {
			t.Fatalf("deadline = %v, want %v", record.Deadline, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPersistentSessionManagerPersistsTransportPeerBinding(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 13, 0, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := kernel.NewPersistentSessionManager(local, store, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "remote task", ContextMode: "isolated", Labels: map[string]string{"task_id": "task-remote", kernel.SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, readErr := r.SubagentRecord(string(id))
		if readErr != nil {
			return readErr
		}
		if record.TransportPeerID != "peer-a" {
			t.Fatalf("transport peer = %q", record.TransportPeerID)
		}
		dispatch, readErr := r.SubagentDispatchByGeneration(string(id), 0)
		if readErr != nil {
			return readErr
		}
		if dispatch.PeerID != "peer-a" || dispatch.Status != domain.SubagentDispatchPending || dispatch.SendAttempt != 0 {
			t.Fatalf("unexpected dispatch: %+v", dispatch)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
