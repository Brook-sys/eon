package kernel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
)

type mockClock struct {
	now time.Time
}

func (c *mockClock) Now() time.Time {
	return c.now
}

func TestLocalSessionManager_ConstructorRequiresClockAndValidPolicy(t *testing.T) {
	_, err := kernel.NewLocalSessionManagerWithPolicy(nil, kernel.SessionPolicy{MaxConcurrent: 1})
	if err == nil {
		t.Fatal("expected error with nil clock")
	}

	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	_, err = kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 0})
	if err == nil {
		t.Fatal("expected error with zero concurrency")
	}
}

func TestLocalSessionManager_SpawnValidatesSpec(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm, _ := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	ctx := context.Background()

	_, err := sm.Spawn(ctx, kernel.SubagentSpec{Task: "", ContextMode: "isolated"})
	if err == nil {
		t.Fatal("expected error missing task")
	}

	_, err = sm.Spawn(ctx, kernel.SubagentSpec{Task: "work", ContextMode: "invalid"})
	if err == nil {
		t.Fatal("expected error invalid context mode")
	}
}

func TestLocalSessionManager_SpawnIdempotencyAndIsolation(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm, _ := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 2})
	ctx := context.Background()
	labels := map[string]string{"task_id": "root-1"}

	id1, err := sm.Spawn(ctx, kernel.SubagentSpec{Task: "T1", ContextMode: "isolated", Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := sm.Spawn(ctx, kernel.SubagentSpec{Task: "T1", ContextMode: "isolated", Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Errorf("expected idempotent ID return %q, got %q", id1, id2)
	}

	_, err = sm.Spawn(ctx, kernel.SubagentSpec{Task: "T1 duplicate", ContextMode: "isolated", Labels: labels})
	if err != kernel.ErrSessionConflict {
		t.Errorf("expected ErrSessionConflict for mismatching spec, got %v", err)
	}

	status, _ := sm.Status(ctx, id1)
	status.Spec.Labels["task_id"] = "tampered"
	status2, _ := sm.Status(ctx, id1)
	if status2.Spec.Labels["task_id"] != "root-1" {
		t.Fatal("expected spec mutation isolation")
	}
}

func TestLocalSessionManager_EnforcesConcurrencyLimit(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm, _ := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	ctx := context.Background()

	_, err := sm.Spawn(ctx, kernel.SubagentSpec{Task: "T1", ContextMode: "isolated", Labels: map[string]string{"task_id": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sm.Spawn(ctx, kernel.SubagentSpec{Task: "T2", ContextMode: "isolated", Labels: map[string]string{"task_id": "2"}})
	if err != kernel.ErrSessionLimit {
		t.Fatalf("expected ErrSessionLimit, got %v", err)
	}
}

func TestLocalSessionManager_RestoreAndPublishTerminalStatus(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 7, 20, 13, 30, 0, 0, time.UTC)}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	status := kernel.SubagentStatus{
		ID: "restored-1", State: kernel.SessionStateRunning,
		Spec:      kernel.SubagentSpec{Task: "continue after restart", ContextMode: "isolated", Labels: map[string]string{"task_id": "task-restored"}},
		StartedAt: clock.Now().Add(-time.Minute),
	}
	if err := manager.Restore(ctx, status); err != nil {
		t.Fatal(err)
	}
	if err := manager.Restore(ctx, status); err != nil {
		t.Fatalf("idempotent restore: %v", err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: status.ID, State: kernel.SessionStateComplete, Result: "done", Failure: ""}); err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: status.ID, State: kernel.SessionStateComplete, Result: "done", Failure: ""}); err != nil {
		t.Fatalf("idempotent terminal replay: %v", err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: status.ID, State: kernel.SessionStateFailed, Result: "", Failure: "late failure"}); !errors.Is(err, kernel.ErrSessionTerminal) {
		t.Fatalf("divergent terminal replay = %v", err)
	}
	got, err := manager.Status(ctx, status.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != kernel.SessionStateComplete || got.Result != "done" || got.Error != nil {
		t.Fatalf("published status = %+v", got)
	}
}

func TestLocalSessionManagerSpawnDoesNotOverwriteRestoredGeneratedID(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 7, 21, 8, 40, 0, 0, time.UTC)}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	restored := kernel.SubagentStatus{
		ID:        "subagent-1",
		State:     kernel.SessionStateRunning,
		Spec:      kernel.SubagentSpec{Task: "restored work", ContextMode: "isolated", Labels: map[string]string{"task_id": "restored-task"}},
		StartedAt: clock.Now().Add(-time.Minute),
	}
	if err := manager.Restore(ctx, restored); err != nil {
		t.Fatal(err)
	}
	spawnedID, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "new work", ContextMode: "isolated", Labels: map[string]string{"task_id": "new-task"}})
	if err != nil {
		t.Fatal(err)
	}
	if spawnedID == restored.ID {
		t.Fatalf("spawn reused restored id %q", restored.ID)
	}
	gotRestored, err := manager.Status(ctx, restored.ID)
	if err != nil {
		t.Fatalf("restored session disappeared: %v", err)
	}
	if gotRestored.Spec.Task != restored.Spec.Task || gotRestored.State != restored.State {
		t.Fatalf("restored session was overwritten: %+v", gotRestored)
	}
	if _, err := manager.Status(ctx, spawnedID); err != nil {
		t.Fatalf("new session missing: %v", err)
	}
}

func TestLocalSessionManager_RollbackSpawnCompensatesPendingOnly(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}

	// Rollback of unknown session returns ErrSessionNotFound.
	if err := manager.RollbackSpawn(ctx, "nonexistent"); !errors.Is(err, kernel.ErrSessionNotFound) {
		t.Fatalf("rollback unknown = %v, want ErrSessionNotFound", err)
	}

	// Spawn a session, verify it exists, then rollback.
	id, err := manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "rollback-me", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "rb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Status(ctx, id); err != nil {
		t.Fatalf("pending session should exist: %v", err)
	}

	// Rollback succeeds and removes the session.
	if err := manager.RollbackSpawn(ctx, id); err != nil {
		t.Fatalf("rollback pending: %v", err)
	}
	if _, err := manager.Status(ctx, id); !errors.Is(err, kernel.ErrSessionNotFound) {
		t.Fatal("session should be removed after rollback")
	}

	// task_id index should also be cleaned — reuse the same task_id.
	idReuse, err := manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "rollback-me-reuse", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "rb-1"},
	})
	if err != nil {
		t.Fatalf("spawn reusing rolled-back task_id should succeed: %v", err)
	}

	// Concurrency slot should be freed: spawn fills second slot.
	idSecond, err := manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "after-rollback-2", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "after-rb-2"},
	})
	if err != nil {
		t.Fatalf("spawn second slot: %v", err)
	}

	// At capacity (2). Complete one to make room for the running test.
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{
		ID: idSecond, State: kernel.SessionStateComplete, Result: "done",
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReleaseTerminal(ctx, idSecond, 0); err != nil {
		t.Fatal(err)
	}

	// Spawn a third session and move it to RUNNING.
	id3, err := manager.Spawn(ctx, kernel.SubagentSpec{
		Task: "running-session", ContextMode: "isolated",
		Labels: map[string]string{"task_id": "running-1"},
	})
	if err != nil {
		t.Fatalf("spawn for running test: %v", err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{
		ID: id3, State: kernel.SessionStateRunning,
	}); err != nil {
		t.Fatal(err)
	}

	// Rollback of a non-pending session must fail closed.
	if err := manager.RollbackSpawn(ctx, id3); !errors.Is(err, kernel.ErrSessionTerminal) {
		t.Fatalf("rollback running = %v, want ErrSessionTerminal", err)
	}

	// Rollback with cancelled context must fail.
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := manager.RollbackSpawn(cancelledCtx, "any"); err == nil {
		t.Fatal("expected error on cancelled context")
	}

	_ = idReuse // used to verify task_id cleanup
}

func TestLocalSessionManager_RetryFailedSessionIsReplaySafe(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC)}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "retry transport", ContextMode: "isolated", Labels: map[string]string{"task_id": "retry-transport"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: id, State: kernel.SessionStateFailed, Result: "", Failure: "temporary"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Retry(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := manager.Retry(ctx, id); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	got, err := manager.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != kernel.SessionStatePending || got.Result != "" || got.Error != nil {
		t.Fatalf("retried status = %+v", got)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: id, Attempt: 1, State: kernel.SessionStateComplete, Result: "done", Failure: ""}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Retry(ctx, id); !errors.Is(err, kernel.ErrSessionTerminal) {
		t.Fatalf("retry completed session = %v", err)
	}
}
