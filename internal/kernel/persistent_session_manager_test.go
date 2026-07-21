package kernel_test

import (
	"context"
	"errors"
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

// failingUpdateStore wraps a real store but injects an error on the first Update
// call, simulating a persistence failure after process-local admission.
type failingUpdateStore struct {
	port.Store
	updateCalls int
	failCount   int
	viewErr     error
}

func (s *failingUpdateStore) Update(ctx context.Context, fn func(port.Transaction) error) error {
	s.updateCalls++
	if s.updateCalls <= s.failCount {
		return errors.New("injected storage failure")
	}
	return s.Store.Update(ctx, fn)
}

func (s *failingUpdateStore) View(ctx context.Context, fn func(port.Reader) error) error {
	if s.viewErr != nil {
		return s.viewErr
	}
	return s.Store.View(ctx, fn)
}

func TestPersistentSessionManagerRollbackOnPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	real := memory.New()
	store := &failingUpdateStore{Store: real, failCount: 1}
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)}
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

	// First spawn: store.Update fails → process-local session should be rolled back.
	_, err = manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "ghost-test", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "ghost-1"},
	})
	if err == nil {
		t.Fatal("expected error from failed persistence")
	}

	// The concurrency slot must have been freed by rollback.
	// Second spawn should succeed (failCount exhausted, Update passes).
	id2, err := manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "survivor", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "survivor-1"},
	})
	if err != nil {
		t.Fatalf("spawn after rollback should succeed: %v", err)
	}

	// Verify the second session was actually persisted.
	if err := real.View(ctx, func(r port.Reader) error {
		record, readErr := r.SubagentRecord(string(id2))
		if readErr != nil {
			return readErr
		}
		if record.State != domain.SubagentStatePending || record.TaskID != "survivor-1" {
			t.Fatalf("unexpected durable record: %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPersistentSessionManagerDoesNotRollbackWhenDurableVerificationFails(t *testing.T) {
	ctx := context.Background()
	verifyFailure := errors.New("injected verification failure")
	store := &failingUpdateStore{Store: memory.New(), failCount: 1, viewErr: verifyFailure}
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 8, 5, 0, 0, time.UTC)}
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

	_, err = manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "ambiguous-store-outcome", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "ambiguous-1"},
	})
	if !errors.Is(err, verifyFailure) {
		t.Fatalf("spawn error = %v, want verification failure", err)
	}

	store.viewErr = nil
	_, err = manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "must-not-use-ambiguous-slot", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "ambiguous-2"},
	})
	if !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn after ambiguous verification = %v, want ErrSessionLimit", err)
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
