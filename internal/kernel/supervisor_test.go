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

func TestSupervisor_Reconcile(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Now()}

	// Use explicit tx to seed pending subagent
	rec1 := domain.SubagentRecord{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "subagent-1",
		TaskID:        "task-1",
		MissionID:     "mission-1",
		State:         domain.SubagentStatePending,
		StartedAt:     clock.Now().Add(-1 * time.Hour),
		UpdatedAt:     clock.Now().Add(-1 * time.Hour),
		Task:          "do something",
		ContextMode:   "isolated",
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateSubagentRecord(rec1)
	}); err != nil {
		t.Fatalf("seed subagent record: %v", err)
	}

	mockManager := &mockSessionManager{
		sessions: map[kernel.SessionID]kernel.SubagentStatus{
			"subagent-1": {
				ID:     "subagent-1",
				State:  kernel.SessionStateComplete,
				Result: "task done",
			},
		},
	}

	supervisor := &kernel.Supervisor{
		Store:   store,
		Manager: mockManager,
		Clock:   clock,
	}

	reconciled, err := supervisor.Reconcile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconciled != 1 {
		t.Errorf("expected 1 reconciled, got %d", reconciled)
	}

	// Verify state changed in store
	store.View(ctx, func(tx port.Reader) error {
		rec, err := tx.SubagentRecord("subagent-1")
		if err != nil {
			t.Errorf("record not found: %v", err)
			return nil
		}
		if rec.State != domain.SubagentStateComplete {
			t.Errorf("expected COMPLETE, got %v", rec.State)
		}
		if rec.Result != "task done" {
			t.Errorf("expected task done, got %v", rec.Result)
		}
		return nil
	})
}

type mockSessionManager struct {
	sessions map[kernel.SessionID]kernel.SubagentStatus
}

func (m *mockSessionManager) Spawn(ctx context.Context, spec kernel.SubagentSpec) (kernel.SessionID, error) {
	return "", errors.New("unimplemented")
}

func (m *mockSessionManager) Status(ctx context.Context, id kernel.SessionID) (kernel.SubagentStatus, error) {
	status, ok := m.sessions[id]
	if !ok {
		return kernel.SubagentStatus{}, kernel.ErrSessionNotFound
	}
	return status, nil
}

func (m *mockSessionManager) Wait(ctx context.Context, id kernel.SessionID) (kernel.SubagentStatus, error) {
	return m.Status(ctx, id)
}

type supervisorMockClock struct {
	currentTime time.Time
}

func (m *supervisorMockClock) Now() time.Time {
	return m.currentTime
}
