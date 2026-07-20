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
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}

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
		IDs:     &supervisorIDs{},
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

type supervisorIDs struct{ n int }

func (i *supervisorIDs) NewID(prefix string) (string, error) {
	i.n++
	return prefix + "-" + string(rune('0'+i.n)), nil
}

type mockSessionManager struct {
	sessions map[kernel.SessionID]kernel.SubagentStatus
}

func (m *mockSessionManager) Spawn(ctx context.Context, spec kernel.SubagentSpec) (kernel.SessionID, error) {
	return "", errors.New("unimplemented")
}

func (m *mockSessionManager) Restore(ctx context.Context, status kernel.SubagentStatus) error {
	m.sessions[status.ID] = status
	return nil
}

func (m *mockSessionManager) PublishStatus(ctx context.Context, id kernel.SessionID, state kernel.SessionState, result, failure string) error {
	status, ok := m.sessions[id]
	if !ok {
		return kernel.ErrSessionNotFound
	}
	status.State = state
	status.Result = result
	if failure != "" {
		status.Error = errors.New(failure)
	}
	m.sessions[id] = status
	return nil
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

func TestSupervisor_RetriesFailedSession(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "retry-1", TaskID: "task-retry", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "retry me", ContextMode: "isolated", Attempt: 0, MaxAttempts: 2}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{"retry-1": {ID: "retry-1", State: kernel.SessionStateFailed, Error: errors.New("transient")}}}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		got, err := tx.SubagentRecord("retry-1")
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStatePending || got.Attempt != 1 || got.ErrorCode != "" {
			t.Fatalf("unexpected retry record: %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisor_ExpiresOrphanedSession(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "orphan-1", TaskID: "task-orphan", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Hour), UpdatedAt: clock.Now().Add(-time.Hour), Task: "stuck", ContextMode: "isolated", Deadline: clock.Now()}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	supervisor := &kernel.Supervisor{Store: store, Manager: &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{}}, Clock: clock}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		got, err := tx.SubagentRecord("orphan-1")
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStateError || got.ErrorCode != "deadline_exceeded" {
			t.Fatalf("unexpected expired record: %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorPersistsTerminalWakeEventExactlyOnce(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "terminal-1", TaskID: "task-terminal", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "finish", ContextMode: "isolated"}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	ids := &supervisorIDs{}
	supervisor := &kernel.Supervisor{Store: store, Manager: &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{"terminal-1": {ID: "terminal-1", State: kernel.SessionStateComplete, Result: "ok"}}}, Clock: clock, IDs: ids}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("first reconcile=(%d,%v)", n, err)
	}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 0 {
		t.Fatalf("replay reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		event, err := tx.ExternalEvent("external-event-1")
		if err != nil {
			return err
		}
		if event.DeduplicationKey != "subagent-terminal:terminal-1" || event.CorrelationID != "terminal-1" || event.Kind != domain.ExternalSubagentCompletion {
			t.Fatalf("unexpected terminal event: %+v", event)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
