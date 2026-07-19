package kernel_test

import (
	"context"
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

func TestLocalSessionManager_SpawnAndStatus(t *testing.T) {
	clock := &mockClock{now: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)}
	sm := kernel.NewLocalSessionManager(clock)

	ctx := context.Background()
	spec := kernel.SubagentSpec{
		Task:        "Analyze dataset",
		ContextMode: "isolated",
	}

	id, err := sm.Spawn(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error spawning subagent: %v", err)
	}

	status, err := sm.Status(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}

	if status.ID != id {
		t.Errorf("expected status ID %q, got %q", id, status.ID)
	}
	if status.State != kernel.SessionStatePending {
		t.Errorf("expected state %q, got %q", kernel.SessionStatePending, status.State)
	}
	if !status.StartedAt.Equal(clock.now) {
		t.Errorf("expected started at %v, got %v", clock.now, status.StartedAt)
	}

	_, err = sm.Status(ctx, "invalid-id")
	if err != kernel.ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}
