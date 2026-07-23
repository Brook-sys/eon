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

func TestPersistentSessionManagerDoesNotPersistRejectedAdmissionAndRecovers(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 23, 12, 5, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := kernel.NewPersistentSessionManager(local, store, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
		MissionID: "mission-limit", MaxAttempts: 2, Timeout: 5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstID, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "hold durable slot", ContextMode: "isolated", Labels: map[string]string{"task_id": "durable-first"}})
	if err != nil {
		t.Fatal(err)
	}
	blockedSpec := kernel.SubagentSpec{Task: "blocked durable admission", ContextMode: "isolated", Labels: map[string]string{"task_id": "durable-blocked"}}
	if _, err := manager.Spawn(ctx, blockedSpec); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn at capacity = %v, want ErrSessionLimit", err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		rows, err := r.SubagentRecordsByState(domain.SubagentStatePending, 0)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].ID != string(firstID) || rows[0].TaskID != "durable-first" {
			t.Fatalf("rejected admission leaked durable state: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
		t.Fatalf("spawn after terminal capacity release: %v", err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(blockedID))
		if err != nil {
			return err
		}
		if record.TaskID != "durable-blocked" || record.Attempt != 0 || record.ErrorCode != "" {
			t.Fatalf("recovered durable admission = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
