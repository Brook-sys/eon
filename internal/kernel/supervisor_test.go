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

func (m *mockSessionManager) RollbackSpawn(context.Context, kernel.SessionID) error { return nil }

func (m *mockSessionManager) Restore(ctx context.Context, status kernel.SubagentStatus) error {
	m.sessions[status.ID] = status
	return nil
}

func (m *mockSessionManager) PublishStatus(ctx context.Context, observation kernel.SubagentObservation) error {
	status, ok := m.sessions[observation.ID]
	if !ok {
		return kernel.ErrSessionNotFound
	}
	status.State = observation.State
	status.Result = observation.Result
	if observation.Failure != "" {
		status.Error = errors.New(observation.Failure)
	}
	m.sessions[observation.ID] = status
	return nil
}

func (m *mockSessionManager) Retry(ctx context.Context, id kernel.SessionID) error {
	status, ok := m.sessions[id]
	if !ok {
		return kernel.ErrSessionNotFound
	}
	if status.State == kernel.SessionStatePending {
		return nil
	}
	if status.State != kernel.SessionStateFailed {
		return kernel.ErrSessionTerminal
	}
	status.State = kernel.SessionStatePending
	status.Attempt++
	status.Result = ""
	status.Error = nil
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
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "retry-1", TaskID: "task-retry", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "retry me", ContextMode: "isolated", TransportPeerID: "peer-a", Attempt: 0, MaxAttempts: 2}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{"retry-1": {ID: "retry-1", State: kernel.SessionStateFailed, Error: errors.New("transient")}}}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
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
	status, err := manager.Status(ctx, "retry-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != kernel.SessionStatePending || status.Error != nil {
		t.Fatalf("transport was not re-armed: %+v", status)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		dispatch, err := tx.SubagentDispatchByGeneration("retry-1", 1)
		if err != nil {
			return err
		}
		if dispatch.PeerID != "peer-a" || dispatch.Status != domain.SubagentDispatchPending {
			t.Fatalf("retry dispatch = %+v", dispatch)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorRecoversRetryRearmedBeforeDurableCommit(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 12, 5, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "retry-split", TaskID: "task-retry-split", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "recover split retry", ContextMode: "isolated", Attempt: 0, MaxAttempts: 2}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{"retry-split": {ID: "retry-split", Attempt: 1, State: kernel.SessionStatePending}}}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		got, err := tx.SubagentRecord("retry-split")
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStatePending || got.Attempt != 1 {
			t.Fatalf("split retry was not completed durably: %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorRecoversRetryCompletedBeforeDurableCommit(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 9, 10, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "retry-fast-complete", TaskID: "task-retry-fast-complete", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "recover fast retry completion", ContextMode: "isolated", Attempt: 0, MaxAttempts: 2, Deadline: clock.Now().Add(time.Minute)}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{
		"retry-fast-complete": {ID: "retry-fast-complete", Attempt: 1, State: kernel.SessionStateComplete, Result: "attempt one result"},
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
		if got.State != domain.SubagentStateComplete || got.Attempt != 1 || got.Result != "attempt one result" {
			t.Fatalf("fast retry completion was not recovered: %+v", got)
		}
		if _, err := tx.ExternalEventByDeduplicationKey("subagent-terminal:" + rec.ID); err != nil {
			t.Fatalf("terminal event missing: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorExpiresRetryAdvancedBeforeDurableCommit(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 9, 15, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "retry-split-deadline", TaskID: "task-retry-split-deadline", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Hour), UpdatedAt: clock.Now().Add(-time.Hour), Task: "expire split retry", ContextMode: "isolated", Attempt: 0, MaxAttempts: 2, Deadline: clock.Now()}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{
		"retry-split-deadline": {ID: "retry-split-deadline", Attempt: 1, State: kernel.SessionStateRunning},
	}}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	status, err := manager.Status(ctx, "retry-split-deadline")
	if err != nil {
		t.Fatal(err)
	}
	if status.Attempt != 1 || status.State != kernel.SessionStateFailed || status.Error == nil || status.Error.Error() != "deadline_exceeded" {
		t.Fatalf("advanced retry was not fenced at deadline: %+v", status)
	}
	if err := store.View(ctx, func(tx port.Reader) error {
		got, err := tx.SubagentRecord(rec.ID)
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStateError || got.Attempt != 1 || got.ErrorCode != "deadline_exceeded" {
			t.Fatalf("durable split retry deadline state=%+v", got)
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

func TestSupervisorDeadlineReleasesManagerConcurrency(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 8, 40, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "expired-active", TaskID: "task-expired-active", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Hour), UpdatedAt: clock.Now().Add(-time.Hour), Task: "stuck but still tracked", ContextMode: "isolated", Attempt: 1, MaxAttempts: 3, Deadline: clock.Now()}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{
		"expired-active": {ID: "expired-active", Attempt: 1, State: kernel.SessionStateRunning},
	}}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	status, err := manager.Status(ctx, "expired-active")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != kernel.SessionStateFailed || status.Error == nil || status.Error.Error() != "deadline_exceeded" {
		t.Fatalf("manager session was not terminalized at deadline: %+v", status)
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
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 0 {
		t.Fatalf("terminal replay=(%d,%v)", n, err)
	}
}

func TestSupervisorTerminalObservationWinsAtDeadline(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 20, 15, 30, 0, 0, time.UTC)}
	rec := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "deadline-complete", TaskID: "task-deadline-complete", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "finish at boundary", ContextMode: "isolated", Attempt: 1, MaxAttempts: 2, Deadline: clock.Now()}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(rec) }); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{
		"deadline-complete": {ID: "deadline-complete", Attempt: 1, State: kernel.SessionStateComplete, Result: "remote result"},
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
		if got.State != domain.SubagentStateComplete || got.Result != "remote result" || got.ErrorCode != "" {
			t.Fatalf("terminal evidence lost at deadline: %+v", got)
		}
		event, err := tx.ExternalEventByDeduplicationKey("subagent-terminal:" + rec.ID)
		if err != nil {
			return err
		}
		if event.Content.Text != "COMPLETE:remote result" {
			t.Fatalf("terminal event=%+v", event)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorIgnoresTerminalReceiptFromPreviousReceiverAttempt(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 12, 20, 0, 0, time.UTC)}
	record := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "receiver-generation", TaskID: "task-generation", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "current attempt", ContextMode: "isolated", Attempt: 1, MaxAttempts: 2, Deadline: clock.Now().Add(time.Minute)}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: domain.SchemaVersionV1, CallerPeerID: "peer-origin", RequestID: "old-request", SourceSessionID: "source-session", Attempt: 0, Task: record.Task, ContextMode: record.ContextMode, ReceiverSessionID: record.ID, ReceiverAttempt: 0, RecordedAt: clock.Now().Add(-time.Minute), Status: domain.SubagentSpawnReceiptPending, UpdatedAt: clock.Now().Add(-time.Minute)}
	terminal, err := domain.CompleteSubagentSpawnReceipt(mustLeaseReceipt(t, receipt, "old-worker", clock.Now().Add(-30*time.Second), clock.Now().Add(30*time.Second)), "old-worker", "stale result", clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(terminal)
	}); err != nil {
		t.Fatal(err)
	}
	manager := &mockSessionManager{sessions: map[kernel.SessionID]kernel.SubagentStatus{kernel.SessionID(record.ID): {ID: kernel.SessionID(record.ID), Attempt: 1, State: kernel.SessionStateRunning}}}
	supervisor := kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 0 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	status, err := manager.Status(ctx, kernel.SessionID(record.ID))
	if err != nil || status.Attempt != 1 || status.State != kernel.SessionStateRunning {
		t.Fatalf("active generation changed=(%+v,%v)", status, err)
	}
}

func TestSupervisorDoesNotReplaceConflictingManagerTerminalFromDurableReceipt(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 21, 12, 40, 0, 0, time.UTC)}
	record := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: "receiver-conflict", TaskID: "task-conflict", MissionID: "mission-1", State: domain.SubagentStateRunning, StartedAt: clock.Now().Add(-time.Minute), UpdatedAt: clock.Now().Add(-time.Minute), Task: "preserve canonical terminal", ContextMode: "isolated", Attempt: 0, MaxAttempts: 1, Deadline: clock.Now().Add(time.Minute)}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: domain.SchemaVersionV1, CallerPeerID: "peer-origin", RequestID: "conflicting-request", SourceSessionID: "source-session", Attempt: 0, Task: record.Task, ContextMode: record.ContextMode, ReceiverSessionID: record.ID, ReceiverAttempt: record.Attempt, RecordedAt: clock.Now().Add(-time.Minute), Status: domain.SubagentSpawnReceiptComplete, UpdatedAt: clock.Now(), Result: "conflicting durable result", StatusDelivery: domain.SubagentStatusDeliveryPending}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	manager := kernel.NewLocalSessionManager(clock)
	status := kernel.SubagentStatus{ID: kernel.SessionID(record.ID), Attempt: record.Attempt, State: kernel.SessionStateRunning, Spec: kernel.SubagentSpec{Task: record.Task, ContextMode: record.ContextMode, Labels: map[string]string{"task_id": record.TaskID}}, StartedAt: record.StartedAt}
	if err := manager.Restore(ctx, status); err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: status.ID, Attempt: status.Attempt, State: kernel.SessionStateComplete, Result: "canonical manager result"}); err != nil {
		t.Fatal(err)
	}
	supervisor := kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	gotStatus, err := manager.Status(ctx, kernel.SessionID(record.ID))
	if err != nil || gotStatus.State != kernel.SessionStateComplete || gotStatus.Result != "canonical manager result" {
		t.Fatalf("manager terminal replaced=(%+v,%v)", gotStatus, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		got, err := r.SubagentRecord(record.ID)
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStateComplete || got.Result != "canonical manager result" {
			t.Fatalf("canonical record=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func mustLeaseReceipt(t *testing.T, receipt domain.SubagentSpawnReceipt, owner string, now, leaseUntil time.Time) domain.SubagentSpawnReceipt {
	t.Helper()
	leased, err := domain.LeaseSubagentSpawnReceipt(receipt, owner, now, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}
	return leased
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
