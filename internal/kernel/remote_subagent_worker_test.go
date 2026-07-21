package kernel_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func seedRemoteReceipt(t *testing.T, store port.Store, manager kernel.SessionManager, now time.Time) domain.SubagentSpawnReceipt {
	t.Helper()
	id, err := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "summarize", ContextMode: "isolated", Labels: map[string]string{"task_id": "receiver-task"}})
	if err != nil {
		t.Fatal(err)
	}
	record := domain.SubagentRecord{SchemaVersion: 1, ID: string(id), TaskID: "receiver-task", MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "summarize", ContextMode: "isolated", MaxAttempts: 3}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-origin", RequestID: "request-1", SourceSessionID: "source-1", Attempt: 2, Task: "summarize", ContextMode: "isolated", ReceiverSessionID: string(id), ReceiverAttempt: record.Attempt, RecordedAt: now, Status: domain.SubagentSpawnReceiptPending, UpdatedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestRemoteSubagentWorkerFencesInactiveReceiverGenerationBeforeExecution(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.SubagentRecord, time.Time)
	}{
		{name: "terminal", mutate: func(record *domain.SubagentRecord, now time.Time) {
			record.State, record.UpdatedAt, record.Result = domain.SubagentStateComplete, now.Add(time.Second), "already done"
		}},
		{name: "replaced attempt", mutate: func(record *domain.SubagentRecord, now time.Time) {
			record.Attempt, record.UpdatedAt = 1, now.Add(time.Second)
		}},
		{name: "deadline reached", mutate: func(record *domain.SubagentRecord, now time.Time) {
			record.Deadline = now
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := memory.New()
			clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
			manager := kernel.NewLocalSessionManager(clock)
			receipt := seedRemoteReceipt(t, store, manager, clock.Now())
			if err := store.Update(context.Background(), func(tx port.Transaction) error {
				record, err := tx.SubagentRecord(receipt.ReceiverSessionID)
				if err != nil {
					return err
				}
				tt.mutate(&record, clock.Now())
				return tx.SaveSubagentRecord(record)
			}); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			worker := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "receiver-worker", Executor: kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
				calls.Add(1)
				return "must not execute", nil
			})}
			if n, err := worker.ExecuteDue(context.Background()); err != nil || n != 1 {
				t.Fatalf("execute=(%d,%v)", n, err)
			}
			if calls.Load() != 0 {
				t.Fatalf("executor calls=%d", calls.Load())
			}
			if err := store.View(context.Background(), func(r port.Reader) error {
				got, err := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
				if err != nil {
					return err
				}
				if got.Status != domain.SubagentSpawnReceiptFailed || got.Failure != "receiver_generation_inactive" || got.StatusDelivery != domain.SubagentStatusDeliveryPending {
					t.Fatalf("receipt=%+v", got)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoteSubagentWorkerExecutesAndCommitsTerminalReceipt(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	manager := kernel.NewLocalSessionManager(clock)
	receipt := seedRemoteReceipt(t, store, manager, clock.Now())
	worker := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "receiver-worker", Lease: time.Minute, Timeout: 30 * time.Second, Executor: kernel.RemoteSubagentExecutorFunc(func(_ context.Context, task, mode string) (string, error) {
		if task != "summarize" || mode != "isolated" {
			t.Fatalf("execution input=(%q,%q)", task, mode)
		}
		return "result evidence", nil
	})}
	if n, err := worker.ExecuteDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("execute=(%d,%v)", n, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if err != nil {
			return err
		}
		if got.Status != domain.SubagentSpawnReceiptComplete || got.Result != "result evidence" || got.StatusDelivery != domain.SubagentStatusDeliveryPending {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(context.Background(), kernel.SessionID(receipt.ReceiverSessionID))
	if err != nil || status.State != kernel.SessionStateComplete || status.Result != "result evidence" {
		t.Fatalf("status=(%+v,%v)", status, err)
	}
}

type publishHookManager struct {
	kernel.SessionManager
	onRunning func(context.Context, kernel.SubagentObservation) error
}

func (m publishHookManager) PublishStatus(ctx context.Context, observation kernel.SubagentObservation) error {
	if observation.State == kernel.SessionStateRunning && m.onRunning != nil {
		if err := m.onRunning(ctx, observation); err != nil {
			return err
		}
	}
	return m.SessionManager.PublishStatus(ctx, observation)
}

func TestRemoteSubagentWorkerFencesGenerationLostAfterClaimBeforeExecution(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	base := kernel.NewLocalSessionManager(clock)
	receipt := seedRemoteReceipt(t, store, base, clock.Now())
	manager := publishHookManager{SessionManager: base, onRunning: func(ctx context.Context, observation kernel.SubagentObservation) error {
		if err := base.PublishStatus(ctx, kernel.SubagentObservation{ID: observation.ID, Attempt: observation.Attempt, State: kernel.SessionStateFailed, Failure: "canonical timeout"}); err != nil {
			return err
		}
		return nil
	}}
	var calls atomic.Int32
	worker := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "receiver-worker", Executor: kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
		calls.Add(1)
		return "must not execute", nil
	})}
	if n, err := worker.ExecuteDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("execute=(%d,%v)", n, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("executor calls=%d", calls.Load())
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if err != nil {
			return err
		}
		if got.Status != domain.SubagentSpawnReceiptFailed || got.Failure != "receiver_generation_inactive_before_execution" || got.StatusDelivery != domain.SubagentStatusDeliveryPending {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSubagentWorkerRejectsResultWhenGenerationEndsDuringExecution(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	manager := kernel.NewLocalSessionManager(clock)
	receipt := seedRemoteReceipt(t, store, manager, clock.Now())
	worker := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "receiver-worker", Lease: time.Minute, Timeout: 30 * time.Second, Executor: kernel.RemoteSubagentExecutorFunc(func(ctx context.Context, task, mode string) (string, error) {
		if err := store.Update(ctx, func(tx port.Transaction) error {
			record, err := tx.SubagentRecord(receipt.ReceiverSessionID)
			if err != nil {
				return err
			}
			record.State, record.UpdatedAt, record.ErrorCode = domain.SubagentStateError, clock.Now().Add(time.Second), "deadline_exceeded"
			return tx.SaveSubagentRecord(record)
		}); err != nil {
			t.Fatal(err)
		}
		clock.currentTime = clock.Now().Add(time.Second)
		return "stale result", nil
	})}
	if n, err := worker.ExecuteDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("execute=(%d,%v)", n, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if err != nil {
			return err
		}
		if got.Status != domain.SubagentSpawnReceiptFailed || got.Failure != "receiver_generation_inactive_after_execution" || got.Result != "" || got.StatusDelivery != domain.SubagentStatusDeliveryPending {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(context.Background(), kernel.SessionID(receipt.ReceiverSessionID))
	if err != nil || status.State != kernel.SessionStateRunning {
		t.Fatalf("status=(%+v,%v)", status, err)
	}
}

func TestRemoteSubagentWorkersDoNotDoubleClaim(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	manager := kernel.NewLocalSessionManager(clock)
	seedRemoteReceipt(t, store, manager, clock.Now())
	var calls atomic.Int32
	executor := kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
		calls.Add(1)
		return "once", nil
	})
	workerA := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "worker-a", Executor: executor}
	workerB := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "worker-b", Executor: executor}
	done := make(chan error, 2)
	go func() { _, err := workerA.ExecuteDue(context.Background()); done <- err }()
	go func() { _, err := workerB.ExecuteDue(context.Background()); done <- err }()
	for range 2 {
		if err := <-done; err != nil && !errors.Is(err, port.ErrConflict) {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("executor calls=%d", calls.Load())
	}
}

func TestRemoteSubagentWorkerParksExpiredLeaseWithoutReexecution(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	manager := kernel.NewLocalSessionManager(clock)
	receipt := seedRemoteReceipt(t, store, manager, clock.Now())
	leased, err := domain.LeaseSubagentSpawnReceipt(receipt, "crashed-worker", clock.Now(), clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(leased, receipt.Status, receipt.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	clock.currentTime = clock.Now().Add(2 * time.Minute)
	worker := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "new-worker", Executor: kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
		t.Fatal("ambiguous expired execution must not be repeated")
		return "", nil
	})}
	if n, err := worker.ExecuteDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("recover=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if got.Status != domain.SubagentSpawnReceiptFailed || got.Failure != "execution_lease_expired_effect_unknown" {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	})
}

type conflictingUpdateStore struct{ port.Store }

func (s conflictingUpdateStore) Update(context.Context, func(port.Transaction) error) error {
	return port.ErrConflict
}

func TestRemoteSubagentWorkerDoesNotPublishExpiredFailureAfterCommitConflict(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	manager := kernel.NewLocalSessionManager(clock)
	receipt := seedRemoteReceipt(t, store, manager, clock.Now())
	leased, err := domain.LeaseSubagentSpawnReceipt(receipt, "crashed-worker", clock.Now(), clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(leased, receipt.Status, receipt.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	clock.currentTime = clock.Now().Add(2 * time.Minute)
	worker := kernel.RemoteSubagentWorker{Store: conflictingUpdateStore{Store: store}, Manager: manager, Clock: clock, Owner: "new-worker", Executor: kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
		t.Fatal("ambiguous expired execution must not be repeated")
		return "", nil
	})}
	if n, err := worker.ExecuteDue(context.Background()); err != nil || n != 0 {
		t.Fatalf("recover=(%d,%v)", n, err)
	}
	status, err := manager.Status(context.Background(), kernel.SessionID(receipt.ReceiverSessionID))
	if err != nil || status.State != kernel.SessionStatePending {
		t.Fatalf("status=(%+v,%v)", status, err)
	}
}

type failedPublishErrorManager struct {
	kernel.SessionManager
	err error
}

func (m failedPublishErrorManager) PublishStatus(ctx context.Context, observation kernel.SubagentObservation) error {
	if observation.State == kernel.SessionStateFailed {
		return m.err
	}
	return m.SessionManager.PublishStatus(ctx, observation)
}

type failTerminalOnceManager struct {
	kernel.SessionManager
	err    error
	failed atomic.Bool
}

func (m *failTerminalOnceManager) PublishStatus(ctx context.Context, observation kernel.SubagentObservation) error {
	if (observation.State == kernel.SessionStateComplete || observation.State == kernel.SessionStateFailed) && m.failed.CompareAndSwap(false, true) {
		return m.err
	}
	return m.SessionManager.PublishStatus(ctx, observation)
}

func TestSupervisorRecoversDurableReceiverTerminalAfterPublicationFailure(t *testing.T) {
	tests := []struct {
		name          string
		executionErr  error
		wantState     domain.SubagentState
		wantReceipt   domain.SubagentSpawnReceiptStatus
		wantResult    string
		wantErrorCode string
	}{
		{name: "complete", wantState: domain.SubagentStateComplete, wantReceipt: domain.SubagentSpawnReceiptComplete, wantResult: "durable evidence"},
		{name: "failed", executionErr: errors.New("provider unavailable"), wantState: domain.SubagentStateError, wantReceipt: domain.SubagentSpawnReceiptFailed, wantErrorCode: "provider unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := memory.New()
			clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
			base := kernel.NewLocalSessionManager(clock)
			receipt := seedRemoteReceipt(t, store, base, clock.Now())
			if err := store.Update(ctx, func(tx port.Transaction) error {
				record, err := tx.SubagentRecord(receipt.ReceiverSessionID)
				if err != nil {
					return err
				}
				record.MaxAttempts = 1
				return tx.SaveSubagentRecord(record)
			}); err != nil {
				t.Fatal(err)
			}
			publishErr := errors.New("manager publication unavailable")
			manager := &failTerminalOnceManager{SessionManager: base, err: publishErr}
			var calls atomic.Int32
			worker := kernel.RemoteSubagentWorker{Store: store, Manager: manager, Clock: clock, Owner: "receiver-worker", Lease: time.Minute, Timeout: 30 * time.Second, Executor: kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
				calls.Add(1)
				return "durable evidence", tt.executionErr
			})}
			if n, err := worker.ExecuteDue(ctx); n != 0 || !errors.Is(err, publishErr) {
				t.Fatalf("execute=(%d,%v)", n, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("executor calls=%d", calls.Load())
			}
			if err := store.View(ctx, func(r port.Reader) error {
				got, err := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
				if err != nil {
					return err
				}
				if got.Status != tt.wantReceipt {
					t.Fatalf("receipt=%+v", got)
				}
				record, err := r.SubagentRecord(receipt.ReceiverSessionID)
				if err != nil {
					return err
				}
				if record.State != domain.SubagentStatePending {
					t.Fatalf("record changed before reconciliation: %+v", record)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			status, err := base.Status(ctx, kernel.SessionID(receipt.ReceiverSessionID))
			if err != nil || status.State != kernel.SessionStateRunning {
				t.Fatalf("status before recovery=(%+v,%v)", status, err)
			}

			supervisor := kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
			if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
				t.Fatalf("reconcile=(%d,%v)", n, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("terminal recovery re-executed work: calls=%d", calls.Load())
			}
			if err := store.View(ctx, func(r port.Reader) error {
				record, err := r.SubagentRecord(receipt.ReceiverSessionID)
				if err != nil {
					return err
				}
				if record.State != tt.wantState || record.Result != tt.wantResult || record.ErrorCode != tt.wantErrorCode {
					t.Fatalf("recovered record=%+v", record)
				}
				if _, err := r.ExternalEventByDeduplicationKey("subagent-terminal:" + receipt.ReceiverSessionID); err != nil {
					return err
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if n, err := supervisor.Reconcile(ctx); err != nil || n != 0 {
				t.Fatalf("terminal replay=(%d,%v)", n, err)
			}
		})
	}
}

func TestRemoteSubagentWorkerSurfacesUnknownExpiredFailurePublicationError(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	base := kernel.NewLocalSessionManager(clock)
	receipt := seedRemoteReceipt(t, store, base, clock.Now())
	leased, err := domain.LeaseSubagentSpawnReceipt(receipt, "crashed-worker", clock.Now(), clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(leased, receipt.Status, receipt.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	clock.currentTime = clock.Now().Add(2 * time.Minute)
	publishErr := errors.New("manager unavailable")
	worker := kernel.RemoteSubagentWorker{Store: store, Manager: failedPublishErrorManager{SessionManager: base, err: publishErr}, Clock: clock, Owner: "new-worker", Executor: kernel.RemoteSubagentExecutorFunc(func(context.Context, string, string) (string, error) {
		t.Fatal("ambiguous expired execution must not be repeated")
		return "", nil
	})}
	if n, err := worker.ExecuteDue(context.Background()); n != 0 || !errors.Is(err, publishErr) {
		t.Fatalf("recover=(%d,%v)", n, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if err != nil {
			return err
		}
		if got.Status != domain.SubagentSpawnReceiptFailed || got.Failure != "execution_lease_expired_effect_unknown" {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
