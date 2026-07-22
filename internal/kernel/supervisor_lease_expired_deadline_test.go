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

func TestSupervisorLeaseExpiredWithDeadline(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)}

	// Test when deadline is reached (it should prioritize deadline_exceeded over lease_expired if both occur,
	// or deadline_exceeded when deadline is before)
	rec := domain.SubagentRecord{
		SchemaVersion:  domain.SchemaVersionV1,
		ID:             "lease-deadline-1",
		TaskID:         "task-lease",
		MissionID:      "mission-1",
		State:          domain.SubagentStateRunning,
		StartedAt:      clock.Now().Add(-2 * time.Hour),
		UpdatedAt:      clock.Now().Add(-2 * time.Hour),
		Task:           "stuck worker",
		ContextMode:    "isolated",
		Attempt:        0,
		MaxAttempts:    3,
		LeaseExpiresAt: clock.Now().Add(-1 * time.Minute),
		Deadline:       clock.Now().Add(-2 * time.Minute),
	}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{
		"lease-deadline-1": {ID: "lease-deadline-1", Attempt: 0, State: kernel.SessionStateRunning},
	}}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		got, err := tx.SubagentRecord(rec.ID)
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStateError || got.ErrorCode != "deadline_exceeded" {
			t.Fatalf("durable deadline state=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
