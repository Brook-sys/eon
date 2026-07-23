package kernel_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

func TestSubagentStatusIngressWorkersConvergeAcrossIndependentSQLiteHandles(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	first, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	clock := &mockClock{now: time.Date(2026, 7, 23, 15, 30, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("local manager: %v", err)
	}
	manager, err := kernel.NewPersistentSessionManager(local, first, clock, &mockIDGenerator{}, kernel.PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 1, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("persistent manager: %v", err)
	}
	sessionID, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "elect sqlite terminal", ContextMode: "isolated"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	recordedAt := clock.now.Add(time.Minute)
	if err := first.Update(ctx, func(tx port.Transaction) error {
		for _, receipt := range []domain.SubagentStatusIngressReceipt{
			{SchemaVersion: 1, CallerPeerID: "peer-a", DeliveryID: "terminal-complete", SessionID: string(sessionID), Attempt: 0, State: "COMPLETE", Result: "winner", Status: domain.SubagentStatusIngressPending, RecordedAt: recordedAt},
			{SchemaVersion: 1, CallerPeerID: "peer-a", DeliveryID: "terminal-failed", SessionID: string(sessionID), Attempt: 0, State: "FAILED", Failure: "divergent", Status: domain.SubagentStatusIngressPending, RecordedAt: recordedAt.Add(time.Second)},
		} {
			if err := tx.CreateSubagentStatusIngressReceipt(receipt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed receipts: %v", err)
	}
	second, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	clock.now = recordedAt.Add(time.Minute)

	workers := []*kernel.SubagentStatusIngressWorker{{Store: first, Manager: manager, Clock: clock, Batch: 2}, {Store: second, Manager: manager, Clock: clock, Batch: 2}}
	start := make(chan struct{})
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, worker := range workers {
		wg.Add(1)
		go func(worker *kernel.SubagentStatusIngressWorker) {
			defer wg.Done()
			<-start
			count, applyErr := worker.ApplyPending(ctx)
			counts <- count
			errs <- applyErr
		}(worker)
	}
	close(start)
	wg.Wait()
	close(counts)
	close(errs)
	processed := 0
	for applyErr := range errs {
		if applyErr != nil {
			t.Fatalf("apply pending: %v", applyErr)
		}
	}
	for count := range counts {
		processed += count
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second: %v", err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if err := reopened.View(ctx, func(r port.Reader) error {
		complete, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-complete")
		if err != nil {
			return err
		}
		failed, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-failed")
		if err != nil {
			return err
		}
		if complete.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("complete = %+v, want APPLIED", complete)
		}
		if failed.Status != domain.SubagentStatusIngressRejected || failed.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Fatalf("failed = %+v, want TERMINAL_CONFLICT", failed)
		}
		pending, err := r.PendingSubagentStatusIngressReceipts(4)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			t.Fatalf("pending after restart = %+v", pending)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify reopen: %v", err)
	}
	if _, err := local.Spawn(ctx, kernel.SubagentSpec{Task: "still held", ContextMode: "isolated"}); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn before acknowledgement = %v, want ErrSessionLimit", err)
	}
	supervisor := &kernel.Supervisor{Store: reopened, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("supervisor reconcile = (%d,%v)", n, err)
	}
	if _, err := local.Spawn(ctx, kernel.SubagentSpec{Task: "released", ContextMode: "isolated"}); err != nil {
		t.Fatalf("spawn after acknowledgement: %v", err)
	}
}
