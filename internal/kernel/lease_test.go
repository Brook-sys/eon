package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestFormatAndParseLeaseDeadline(t *testing.T) {
	t.Parallel()
	until := time.Date(2026, 7, 16, 12, 15, 0, 123456789, time.UTC)
	ref := FormatLeaseRef("lease_1", "operation_a", 2, until)
	got, ok := ParseLeaseDeadline(ref)
	if !ok {
		t.Fatalf("expected deadline in %q", ref)
	}
	if got.UTC().UnixNano() != until.UTC().UnixNano() {
		t.Fatalf("deadline = %v, want %v (ref=%s)", got, until, ref)
	}
	if _, ok := ParseLeaseDeadline("lease_1:op=operation_a:attempt=1"); ok {
		t.Fatal("legacy ref without until must not invent a deadline")
	}
	if LeaseExpired(domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: ref}, until.Add(-time.Second)) {
		t.Fatal("lease must not be expired before deadline")
	}
	if !LeaseExpired(domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: ref}, until) {
		t.Fatal("lease must be expired at deadline")
	}
}

func TestLeaseReaperMovesExpiredRunningToReady(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	start := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(start)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedAgenda(t, store, start)

	// Force operation_a into RUNNING with an already-expired lease.
	expiredUntil := start.Add(-time.Minute)
	leaseRef := FormatLeaseRef("lease_old", "operation_a", 1, expiredUntil)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_a")
		if err != nil {
			return err
		}
		op.State = domain.StateRunning
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: leaseRef}
		op.Attempt = 1
		return tx.SaveOperation(op)
	}); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	reaper := LeaseReaper{Store: store, Clock: clock, IDs: ids}
	result, err := reaper.Reconcile(ctx, "revision_1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Reconciled != 1 {
		t.Fatalf("reconciled = %+v", result)
	}

	var op domain.Operation
	var events []domain.Event
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_a")
		if err != nil {
			return err
		}
		events, err = r.Events(0, 50)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateReady {
		t.Fatalf("state = %s, want READY after reconcile", op.State)
	}
	kinds := map[string]int{}
	for _, event := range events {
		if event.OperationID == "operation_a" {
			kinds[event.Kind]++
		}
	}
	if kinds[EventOperationLeaseExpired] != 1 || kinds[EventOperationReconciling] != 1 {
		t.Fatalf("events = %v", kinds)
	}

	// Fresh lease must not be reaped.
	fresh := FormatLeaseRef("lease_new", "operation_a", 2, start.Add(10*time.Minute))
	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_a")
		if err != nil {
			return err
		}
		op.State = domain.StateVerifying
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: fresh}
		return tx.SaveOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	again, err := reaper.Reconcile(ctx, "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if again.Reconciled != 0 {
		t.Fatalf("fresh lease should not reconcile: %+v", again)
	}
}
