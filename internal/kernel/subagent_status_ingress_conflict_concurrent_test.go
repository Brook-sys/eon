package kernel_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestSubagentStatusIngressWorkerLimitsConflictsConcurrently(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := memory.New()

	clock := &mockClock{now: time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)}
	ids := &mockIDGenerator{}

	localManager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 4})
	if err != nil {
		t.Fatalf("local manager: %v", err)
	}

	manager, err := kernel.NewPersistentSessionManager(localManager, store, clock, ids, kernel.PersistentSessionPolicy{MissionID: "mission-1"})
	if err != nil {
		t.Fatalf("persistent manager: %v", err)
	}

	worker := &kernel.SubagentStatusIngressWorker{
		Store:   store,
		Manager: manager,
		Clock:   clock,
		Batch:   10,
	}

	spec := kernel.SubagentSpec{
		Task:        "work",
		ContextMode: "isolated",
	}

	rawSessionID, err := manager.Spawn(ctx, spec)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sessionID := string(rawSessionID)
	now := clock.now.Add(time.Minute)

	// Inject conflicting RUNNING -> COMPLETE -> RUNNING sequence to pending queue
	runReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "run-1",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "RUNNING",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    now,
	}
	completeReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "complete-1",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "COMPLETE",
		Result:        "done-first",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    now.Add(time.Second),
	}
	// A late running receipt that arrives out of order
	lateRunReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "run-2-late",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "RUNNING",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    now.Add(2 * time.Second),
	}
	// A conflicting complete receipt
	conflictCompleteReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "complete-2-conflict",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "COMPLETE",
		Result:        "done-second",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    now.Add(3 * time.Second),
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateSubagentStatusIngressReceipt(runReceipt); err != nil {
			return err
		}
		if err := tx.CreateSubagentStatusIngressReceipt(completeReceipt); err != nil {
			return err
		}
		if err := tx.CreateSubagentStatusIngressReceipt(lateRunReceipt); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(conflictCompleteReceipt)
	}); err != nil {
		t.Fatalf("store receipts: %v", err)
	}

	clock.now = clock.now.Add(2 * time.Minute)

	// Since pending is sorted implicitly by RecordedAt or retrieval order (memory store PendingSubagentStatusIngressReceipts),
	// applying the batch should correctly apply the first RUNNING and COMPLETE, then reject the latter two.
	processed, err := worker.ApplyPending(ctx)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if processed != 4 {
		t.Errorf("expected 4 processed, got %d", processed)
	}

	// Verify terminal state wasn't mutated
	status, err := manager.Wait(ctx, rawSessionID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if status.State != kernel.SessionStateComplete || status.Result != "done-first" {
		t.Errorf("terminal state mutated: %+v", status)
	}

	if err := store.View(ctx, func(r port.Reader) error {
		r1, _ := r.SubagentStatusIngressReceipt("peer-a", "run-1")
		if r1.Status != domain.SubagentStatusIngressApplied {
			t.Errorf("run-1 expected APPLIED, got %v", r1.Status)
		}

		r2, _ := r.SubagentStatusIngressReceipt("peer-a", "complete-1")
		if r2.Status != domain.SubagentStatusIngressApplied {
			t.Errorf("complete-1 expected APPLIED, got %v", r2.Status)
		}

		r3, _ := r.SubagentStatusIngressReceipt("peer-a", "run-2-late")
		if r3.Status != domain.SubagentStatusIngressRejected || r3.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Errorf("run-2-late expected REJECTED with TERMINAL_CONFLICT, got %v %v", r3.Status, r3.RejectionCode)
		}

		r4, _ := r.SubagentStatusIngressReceipt("peer-a", "complete-2-conflict")
		if r4.Status != domain.SubagentStatusIngressRejected || r4.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Errorf("complete-2-conflict expected REJECTED with TERMINAL_CONFLICT, got %v %v", r4.Status, r4.RejectionCode)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify storage: %v", err)
	}
}

func TestSubagentStatusIngressWorkersCountEachPendingReceiptOnce(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &mockClock{now: time.Date(2026, 7, 23, 14, 30, 0, 0, time.UTC)}
	ids := &mockIDGenerator{}

	localManager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("local manager: %v", err)
	}
	manager, err := kernel.NewPersistentSessionManager(localManager, store, clock, ids, kernel.PersistentSessionPolicy{MissionID: "mission-1"})
	if err != nil {
		t.Fatalf("persistent manager: %v", err)
	}
	sessionID, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "work", ContextMode: "isolated"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	receipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "terminal-1",
		SessionID:     string(sessionID),
		Attempt:       0,
		State:         "COMPLETE",
		Result:        "winner",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    clock.now.Add(time.Minute),
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateSubagentStatusIngressReceipt(receipt)
	}); err != nil {
		t.Fatalf("store receipt: %v", err)
	}
	clock.now = receipt.RecordedAt.Add(time.Minute)

	workers := []*kernel.SubagentStatusIngressWorker{
		{Store: store, Manager: manager, Clock: clock, Batch: 1},
		{Store: store, Manager: manager, Clock: clock, Batch: 1},
	}
	start := make(chan struct{})
	counts := make(chan int, len(workers))
	errs := make(chan error, len(workers))
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

	total := 0
	for applyErr := range errs {
		if applyErr != nil {
			t.Fatalf("apply pending: %v", applyErr)
		}
	}
	for count := range counts {
		total += count
	}
	if total != 1 {
		t.Fatalf("expected one durable pending-to-applied transition, got processed total %d", total)
	}
	status, err := manager.Wait(ctx, sessionID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if status.State != kernel.SessionStateComplete || status.Result != "winner" {
		t.Fatalf("unexpected elected terminal status: %+v", status)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		got, readErr := r.SubagentStatusIngressReceipt("peer-a", "terminal-1")
		if readErr != nil {
			return readErr
		}
		if got.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("expected APPLIED receipt, got %s", got.Status)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify receipt: %v", err)
	}
	if _, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "must remain backpressured", ContextMode: "isolated"}); err != kernel.ErrSessionLimit {
		t.Fatalf("terminal capacity released before canonical acknowledgement: got %v", err)
	}
}

func TestSubagentStatusIngressWorkersConvergeDivergentTerminalsAndReleaseExactGeneration(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &mockClock{now: time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC)}
	ids := &mockIDGenerator{}

	localManager, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("local manager: %v", err)
	}
	manager, err := kernel.NewPersistentSessionManager(localManager, store, clock, ids, kernel.PersistentSessionPolicy{
		MissionID:   "mission-1",
		MaxAttempts: 1,
		Timeout:     time.Minute,
	})
	if err != nil {
		t.Fatalf("persistent manager: %v", err)
	}
	sessionID, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "elect concurrent terminal", ContextMode: "isolated"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	recordedAt := clock.now.Add(time.Minute)
	receipts := []domain.SubagentStatusIngressReceipt{
		{
			SchemaVersion: 1,
			CallerPeerID:  "peer-a",
			DeliveryID:    "terminal-complete",
			SessionID:     string(sessionID),
			Attempt:       0,
			State:         "COMPLETE",
			Result:        "winner",
			Status:        domain.SubagentStatusIngressPending,
			RecordedAt:    recordedAt,
		},
		{
			SchemaVersion: 1,
			CallerPeerID:  "peer-a",
			DeliveryID:    "terminal-failed",
			SessionID:     string(sessionID),
			Attempt:       0,
			State:         "FAILED",
			Failure:       "divergent",
			Status:        domain.SubagentStatusIngressPending,
			RecordedAt:    recordedAt.Add(time.Second),
		},
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		for _, receipt := range receipts {
			if err := tx.CreateSubagentStatusIngressReceipt(receipt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("store receipts: %v", err)
	}
	clock.now = recordedAt.Add(time.Minute)

	workers := []*kernel.SubagentStatusIngressWorker{
		{Store: store, Manager: manager, Clock: clock, Batch: 2},
		{Store: store, Manager: manager, Clock: clock, Batch: 2},
	}
	start := make(chan struct{})
	counts := make(chan int, len(workers))
	errs := make(chan error, len(workers))
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
		t.Fatalf("expected exactly two durable pending transitions, got %d", processed)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		complete, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-complete")
		if err != nil {
			return err
		}
		failed, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-failed")
		if err != nil {
			return err
		}
		if complete.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("complete receipt = %+v, want APPLIED", complete)
		}
		if failed.Status != domain.SubagentStatusIngressRejected || failed.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Fatalf("failed receipt = %+v, want TERMINAL_CONFLICT", failed)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify receipts: %v", err)
	}
	status, err := manager.Wait(ctx, sessionID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if status.State != kernel.SessionStateComplete || status.Result != "winner" {
		t.Fatalf("elected terminal = %+v", status)
	}
	blocked := kernel.SubagentSpec{Task: "wait for exact terminal acknowledgement", ContextMode: "isolated"}
	if _, err := manager.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn before canonical acknowledgement = %v, want ErrSessionLimit", err)
	}

	if err := manager.ReleaseTerminal(ctx, sessionID, 1); !errors.Is(err, kernel.ErrSessionAttempt) {
		t.Fatalf("wrong-generation release = %v, want ErrSessionAttempt", err)
	}
	if _, err := manager.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn after wrong-generation release = %v, want ErrSessionLimit", err)
	}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("supervisor reconcile=(%d,%v)", n, err)
	}
	if _, err := manager.Spawn(ctx, blocked); err != nil {
		t.Fatalf("spawn after canonical acknowledgement: %v", err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(sessionID))
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStateComplete || record.Attempt != 0 || record.Result != "winner" {
			t.Fatalf("canonical terminal = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify canonical terminal: %v", err)
	}
}
