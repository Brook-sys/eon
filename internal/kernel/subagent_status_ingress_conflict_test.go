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

type mockIDGenerator struct{ count int }

func (g *mockIDGenerator) NewID(prefix string) (string, error) {
	g.count++
	return prefix, nil
}

func TestSubagentStatusIngressWorkerLimitsConflictsAndMaintainsIdempotentState(t *testing.T) {
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

	// Ingress receipts must be after record's start time and within limits
	now := clock.now.Add(time.Minute)

	// 1. Simulate multiple status receipts arriving concurrently or sequentially 
	// for the same attempt. One is RUNNING, another is COMPLETE.
	runningReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "delivery-run-1",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "RUNNING",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    now,
	}
	completeReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "delivery-complete-1",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "COMPLETE",
		Result:        "done-1",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    now.Add(time.Second),
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateSubagentStatusIngressReceipt(runningReceipt); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(completeReceipt)
	}); err != nil {
		t.Fatalf("store receipts: %v", err)
	}

	clock.now = now.Add(2 * time.Second) // Advance clock for application time

	processed, err := worker.ApplyPending(ctx)
	if err != nil {
		t.Fatalf("apply 1: %v", err)
	}
	if processed != 2 {
		t.Errorf("expected 2 processed, got %d", processed)
	}

	status, err := manager.Wait(ctx, rawSessionID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if status.State != kernel.SessionStateComplete || status.Result != "done-1" {
		t.Errorf("unexpected status: %+v", status)
	}

	// 2. Late arriving receipts for attempt 0 after session is COMPLETE
	// Should be rejected due to TERMINAL_CONFLICT
	lateRunReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "delivery-run-late",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "RUNNING",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    clock.now,
	}
	lateCompleteMismatchReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "delivery-complete-late",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "COMPLETE",
		Result:        "done-different", // Different result conflicts with terminal state
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    clock.now.Add(time.Second),
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateSubagentStatusIngressReceipt(lateRunReceipt); err != nil {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(lateCompleteMismatchReceipt)
	}); err != nil {
		t.Fatalf("store late receipts: %v", err)
	}

	clock.now = clock.now.Add(2 * time.Second)

	processed, err = worker.ApplyPending(ctx)
	if err != nil {
		t.Fatalf("apply late: %v", err)
	}
	if processed != 2 {
		t.Errorf("expected 2 processed late receipts, got %d", processed)
	}

	// Verify terminal state wasn't mutated
	status, err = manager.Wait(ctx, rawSessionID)
	if err != nil {
		t.Fatalf("wait after late: %v", err)
	}
	if status.State != kernel.SessionStateComplete || status.Result != "done-1" {
		t.Errorf("terminal state mutated: %+v", status)
	}

	// Verify late receipts were rejected
	if err := store.View(ctx, func(r port.Reader) error {
		r1, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-run-late")
		if err != nil {
			return err
		}
		if r1.Status != domain.SubagentStatusIngressRejected || r1.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Errorf("expected late run receipt to be rejected with terminal conflict, got status %v code %v", r1.Status, r1.RejectionCode)
		}

		r2, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-complete-late")
		if err != nil {
			return err
		}
		if r2.Status != domain.SubagentStatusIngressRejected || r2.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Errorf("expected late complete receipt to be rejected with terminal conflict, got status %v code %v", r2.Status, r2.RejectionCode)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify rejections: %v", err)
	}

	// 3. Receipt for mismatched attempt (attempt 1 while local manager is on attempt 0, or after terminal)
	attemptMismatchReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "delivery-mismatch",
		SessionID:     sessionID,
		Attempt:       1,
		State:         "RUNNING",
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    clock.now,
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateSubagentStatusIngressReceipt(attemptMismatchReceipt)
	}); err != nil {
		t.Fatalf("store mismatch: %v", err)
	}

	clock.now = clock.now.Add(time.Second)

	processed, err = worker.ApplyPending(ctx)
	if err != nil {
		t.Fatalf("apply mismatch: %v", err)
	}
	if processed != 1 {
		t.Errorf("expected 1 processed mismatch receipt, got %d", processed)
	}

	if err := store.View(ctx, func(r port.Reader) error {
		r1, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-mismatch")
		if err != nil {
			return err
		}
		if r1.Status != domain.SubagentStatusIngressRejected || r1.RejectionCode != domain.SubagentStatusIngressRejectionAttemptMismatch {
			t.Errorf("expected mismatch receipt to be rejected with attempt mismatch, got status %v code %v", r1.Status, r1.RejectionCode)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify mismatch rejection: %v", err)
	}
	
	// 4. Verify idempotent apply for identical COMPLETE (manager returns nil, should mark as APPLIED)
	identicalReceipt := domain.SubagentStatusIngressReceipt{
		SchemaVersion: 1,
		CallerPeerID:  "peer-a",
		DeliveryID:    "delivery-complete-identical",
		SessionID:     sessionID,
		Attempt:       0,
		State:         "COMPLETE",
		Result:        "done-1", // Same result
		Status:        domain.SubagentStatusIngressPending,
		RecordedAt:    clock.now,
	}
	
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateSubagentStatusIngressReceipt(identicalReceipt)
	}); err != nil {
		t.Fatalf("store identical: %v", err)
	}

	clock.now = clock.now.Add(time.Second)

	processed, err = worker.ApplyPending(ctx)
	if err != nil {
		t.Fatalf("apply identical: %v", err)
	}
	if processed != 1 {
		t.Errorf("expected 1 processed identical receipt, got %d", processed)
	}
	
	if err := store.View(ctx, func(r port.Reader) error {
		r1, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-complete-identical")
		if err != nil {
			return err
		}
		if r1.Status != domain.SubagentStatusIngressApplied {
			t.Errorf("expected identical receipt to be marked APPLIED idempotently, got status %v", r1.Status)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify identical application: %v", err)
	}
}
