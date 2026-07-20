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
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-origin", RequestID: "request-1", SourceSessionID: "source-1", Attempt: 2, Task: "summarize", ContextMode: "isolated", ReceiverSessionID: string(id), RecordedAt: now, Status: domain.SubagentSpawnReceiptPending, UpdatedAt: now}
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
