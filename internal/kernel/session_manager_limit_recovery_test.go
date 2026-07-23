package kernel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
)

func TestLocalSessionManagerAdmissionLimitIsSideEffectFreeAndRecovers(t *testing.T) {
	ctx := context.Background()
	clock := &mockClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)}
	manager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}

	firstSpec := kernel.SubagentSpec{Task: "hold only slot", ContextMode: "isolated", Labels: map[string]string{"task_id": "limit-first"}}
	firstID, err := manager.Spawn(ctx, firstSpec)
	if err != nil {
		t.Fatal(err)
	}
	blockedSpec := kernel.SubagentSpec{Task: "wait for capacity", ContextMode: "isolated", Labels: map[string]string{"task_id": "limit-blocked"}}
	if _, err := manager.Spawn(ctx, blockedSpec); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn at capacity = %v, want ErrSessionLimit", err)
	}

	first, err := manager.Status(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != kernel.SessionStatePending || first.Attempt != 0 || first.Error != nil {
		t.Fatalf("capacity rejection mutated admitted session: %+v", first)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: firstID, Attempt: 0, State: kernel.SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Spawn(ctx, blockedSpec); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn before terminal durability acknowledgement = %v, want ErrSessionLimit", err)
	}
	if err := manager.ReleaseTerminal(ctx, firstID, 0); err != nil {
		t.Fatal(err)
	}

	blockedID, err := manager.Spawn(ctx, blockedSpec)
	if err != nil {
		t.Fatalf("spawn after capacity release: %v", err)
	}
	if blockedID == "" || blockedID == firstID {
		t.Fatalf("spawn after release returned invalid id %q", blockedID)
	}
	blocked, err := manager.Status(ctx, blockedID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != kernel.SessionStatePending || blocked.Attempt != 0 || blocked.Error != nil {
		t.Fatalf("recovered admission status = %+v", blocked)
	}
}
