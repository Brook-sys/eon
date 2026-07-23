package kernel

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/storage/memory"
)

type ingressWorkerClock struct{ now time.Time }

func (c ingressWorkerClock) Now() time.Time { return c.now }

type ingressWorkerIDs struct{ n int }

func (i *ingressWorkerIDs) NewID(prefix string) (string, error) {
	i.n++
	return prefix + "-" + strconv.Itoa(i.n), nil
}

type ingressWorkerFailingManager struct {
	SessionManager
	err error
}

type ingressWorkerConflictStore struct {
	port.Store
	conflicts int
	updates   int
}

func (s *ingressWorkerConflictStore) Update(ctx context.Context, fn func(port.Transaction) error) error {
	s.updates++
	if s.conflicts > 0 {
		s.conflicts--
		return port.ErrConflict
	}
	return s.Store.Update(ctx, fn)
}

type ingressWorkerSleeper struct{ delays []time.Duration }

func (s *ingressWorkerSleeper) Sleep(_ context.Context, delay time.Duration) error {
	s.delays = append(s.delays, delay)
	return nil
}

func (m ingressWorkerFailingManager) PublishStatus(context.Context, SubagentObservation) error {
	return m.err
}

func TestSubagentStatusIngressWorkerApplyRestartAndConflict(t *testing.T) {
	ctx := context.Background()
	clock := ingressWorkerClock{now: time.Unix(100, 0).UTC()}
	store := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, store, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, SubagentSpec{Task: "work", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-1", SubagentObservation{ID: id, State: SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}
	worker := SubagentStatusIngressWorker{Store: store, Manager: manager, Clock: clock, Batch: 1}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 1 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	status, _ := manager.Status(ctx, id)
	if status.State != SessionStateComplete || status.Result != "done" {
		t.Fatalf("status=%+v", status)
	}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 0 {
		t.Fatalf("restart n=%d err=%v", n, err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-1", SubagentObservation{ID: id, State: SessionStateComplete, Result: "other"}); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("conflict=%v", err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		receipt, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-1")
		if err != nil {
			return err
		}
		if receipt.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("receipt=%+v", receipt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusIngressWorkerRetriesIdempotentReceiptCASBoundedly(t *testing.T) {
	ctx := context.Background()
	clock := ingressWorkerClock{now: time.Unix(125, 0).UTC()}
	base := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, base, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-retry", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, SubagentSpec{Task: "retry receipt cas", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-retry", SubagentObservation{ID: id, State: SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}
	store := &ingressWorkerConflictStore{Store: base, conflicts: 2}
	sleeper := &ingressWorkerSleeper{}
	var report retry.Report
	worker := SubagentStatusIngressWorker{
		Store: store, Manager: manager, Clock: clock, Batch: 1,
		RetryPolicy:  retry.Policy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond},
		RetrySleeper: sleeper,
		RetryObserve: func(observed retry.Report) { report = observed },
	}
	n, aggregate, err := worker.ApplyPendingWithRetryReport(ctx)
	if err != nil || n != 1 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	if store.updates != 3 || report.Attempts != 3 || report.Retries != 2 || report.Classes["conflict"] != 2 {
		t.Fatalf("updates=%d report=%+v", store.updates, report)
	}
	if len(sleeper.delays) != 2 || sleeper.delays[0] != time.Millisecond || sleeper.delays[1] != 2*time.Millisecond {
		t.Fatalf("delays=%v", sleeper.delays)
	}
	if aggregate.Attempts != 3 || aggregate.Retries != 2 || aggregate.Classes["conflict"] != 2 || aggregate.SleepTotal != 3*time.Millisecond {
		t.Fatalf("aggregate=%+v", aggregate)
	}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 0 {
		t.Fatalf("replay n=%d err=%v", n, err)
	}
}

func TestSubagentStatusIngressWorkerRetryBudgetExhaustionLeavesReceiptPending(t *testing.T) {
	ctx := context.Background()
	clock := ingressWorkerClock{now: time.Unix(130, 0).UTC()}
	base := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, base, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-exhaustion", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, SubagentSpec{Task: "exhaust receipt cas", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-exhaust", SubagentObservation{ID: id, State: SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}
	store := &ingressWorkerConflictStore{Store: base, conflicts: 3}
	worker := SubagentStatusIngressWorker{
		Store: store, Manager: manager, Clock: clock, Batch: 1,
		RetryPolicy: retry.Policy{MaxAttempts: 2}, RetrySleeper: &ingressWorkerSleeper{},
	}
	if n, err := worker.ApplyPending(ctx); n != 0 || !errors.Is(err, retry.ErrBudgetExhausted) || !errors.Is(err, port.ErrConflict) {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	if err := base.View(ctx, func(r port.Reader) error {
		receipt, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-exhaust")
		if err != nil {
			return err
		}
		if receipt.Status != domain.SubagentStatusIngressPending {
			t.Fatalf("receipt=%+v", receipt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusIngressWorkerRenewsLeaseFromAuthenticatedRunningHeartbeat(t *testing.T) {
	ctx := context.Background()
	clock := &ingressWorkerClock{now: time.Unix(150, 0).UTC()}
	store := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, store, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-lease", MaxAttempts: 2, Timeout: 10 * time.Minute, LeaseTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, SubagentSpec{Task: "long work", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(45 * time.Second)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "heartbeat-1", SubagentObservation{ID: id, Attempt: 0, State: SessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	worker := SubagentStatusIngressWorker{Store: store, Manager: manager, Clock: clock, Batch: 1, LeaseTTL: time.Minute}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 1 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(id))
		if err != nil {
			return err
		}
		want := clock.Now().Add(time.Minute)
		if record.State != domain.SubagentStatePending || !record.LeaseExpiresAt.Equal(want) || !record.UpdatedAt.Equal(clock.Now()) {
			t.Fatalf("record=%+v want lease=%v", record, want)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentStatusIngressWorkerQuarantinesAttemptMismatchAndContinues(t *testing.T) {
	ctx := context.Background()
	clock := ingressWorkerClock{now: time.Unix(200, 0).UTC()}
	store := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, store, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, SubagentSpec{Task: "work", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, SubagentObservation{ID: id, Attempt: 0, State: SessionStateFailed, Failure: "retry"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Retry(ctx, id); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-stale", SubagentObservation{ID: id, Attempt: 0, State: SessionStateComplete, Result: "late"}); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-current", SubagentObservation{ID: id, Attempt: 1, State: SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}

	worker := SubagentStatusIngressWorker{Store: store, Manager: manager, Clock: clock, Batch: 2}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 2 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	status, err := manager.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != SessionStateComplete || status.Attempt != 1 || status.Result != "done" {
		t.Fatalf("status=%+v", status)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		stale, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-stale")
		if err != nil {
			return err
		}
		current, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-current")
		if err != nil {
			return err
		}
		if stale.Status != domain.SubagentStatusIngressRejected || stale.RejectionCode != domain.SubagentStatusIngressRejectionAttemptMismatch || current.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("stale=%+v current=%+v", stale, current)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 0 {
		t.Fatalf("restart n=%d err=%v", n, err)
	}
}

func TestSubagentStatusIngressWorkerQuarantinesTerminalConflictAndContinues(t *testing.T) {
	ctx := context.Background()
	clock := ingressWorkerClock{now: time.Unix(300, 0).UTC()}
	store := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, store, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	conflictedID, err := manager.Spawn(ctx, SubagentSpec{Task: "conflicted", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	validID, err := manager.Spawn(ctx, SubagentSpec{Task: "valid", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-a-winner", SubagentObservation{ID: conflictedID, Attempt: 0, State: SessionStateComplete, Result: "canonical"}); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-b-conflict", SubagentObservation{ID: conflictedID, Attempt: 0, State: SessionStateFailed, Failure: "contradiction"}); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-c-replay", SubagentObservation{ID: conflictedID, Attempt: 0, State: SessionStateComplete, Result: "canonical"}); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(time.Second)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-d-valid", SubagentObservation{ID: validID, Attempt: 0, State: SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}

	worker := SubagentStatusIngressWorker{Store: store, Manager: manager, Clock: clock, Batch: 4}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 4 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		winner, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-a-winner")
		if err != nil {
			return err
		}
		conflicted, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-b-conflict")
		if err != nil {
			return err
		}
		replay, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-c-replay")
		if err != nil {
			return err
		}
		valid, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-d-valid")
		if err != nil {
			return err
		}
		if winner.Status != domain.SubagentStatusIngressApplied || conflicted.Status != domain.SubagentStatusIngressRejected || conflicted.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict || replay.Status != domain.SubagentStatusIngressApplied || valid.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("winner=%+v conflicted=%+v replay=%+v valid=%+v", winner, conflicted, replay, valid)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(ctx, conflictedID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != SessionStateComplete || status.Result != "canonical" {
		t.Fatalf("terminal status mutated: %+v", status)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-b-conflict", SubagentObservation{ID: conflictedID, Attempt: 0, State: SessionStateFailed, Failure: "contradiction"}); err != nil {
		t.Fatalf("identical rejected replay: %v", err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-b-conflict", SubagentObservation{ID: conflictedID, Attempt: 0, State: SessionStateFailed, Failure: "different"}); !errors.Is(err, ErrSessionConflict) {
		t.Fatalf("divergent rejected replay: %v", err)
	}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 0 {
		t.Fatalf("restart n=%d err=%v", n, err)
	}
}

func TestSubagentStatusIngressWorkerLeavesUnknownFailurePending(t *testing.T) {
	ctx := context.Background()
	clock := ingressWorkerClock{now: time.Unix(400, 0).UTC()}
	store := memory.New()
	local := NewLocalSessionManager(clock)
	manager, err := NewPersistentSessionManager(local, store, clock, &ingressWorkerIDs{}, PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, SubagentSpec{Task: "work", ContextMode: "isolated", Labels: map[string]string{SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-pending", SubagentObservation{ID: id, State: SessionStateComplete, Result: "done"}); err != nil {
		t.Fatal(err)
	}
	managerFailure := errors.New("manager unavailable")
	worker := SubagentStatusIngressWorker{Store: store, Manager: ingressWorkerFailingManager{SessionManager: manager, err: managerFailure}, Clock: clock, Batch: 1}
	if n, err := worker.ApplyPending(ctx); n != 0 || !errors.Is(err, managerFailure) {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		receipt, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-pending")
		if err != nil {
			return err
		}
		if receipt.Status != domain.SubagentStatusIngressPending {
			t.Fatalf("receipt=%+v", receipt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
