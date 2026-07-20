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
