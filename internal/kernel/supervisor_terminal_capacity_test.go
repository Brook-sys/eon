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

type rollbackAfterSupervisorUpdateStore struct {
	port.Store
	fail bool
}

type failOnceReleaseSessionManager struct {
	kernel.SessionManager
	err error
}

func (m *failOnceReleaseSessionManager) ReleaseTerminal(ctx context.Context, id kernel.SessionID, attempt int) error {
	if m.err != nil {
		err := m.err
		m.err = nil
		return err
	}
	return m.SessionManager.ReleaseTerminal(ctx, id, attempt)
}

func (s *rollbackAfterSupervisorUpdateStore) Update(ctx context.Context, fn func(port.Transaction) error) error {
	return s.Store.Update(ctx, func(tx port.Transaction) error {
		if err := fn(tx); err != nil {
			return err
		}
		if s.fail {
			s.fail = false
			return errors.New("injected supervisor commit failure")
		}
		return nil
	})
}

func TestSupervisorTerminalFenceHoldsCapacityUntilCanonicalCommit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		leaseTTL  time.Duration
		advance   time.Duration
		errorCode string
	}{
		{name: "deadline", advance: time.Minute, errorCode: "deadline_exceeded"},
		{name: "lease", leaseTTL: 30 * time.Second, advance: 30 * time.Second, errorCode: "lease_expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testSupervisorTerminalFenceHoldsCapacityUntilCanonicalCommit(t, tc.leaseTTL, tc.advance, tc.errorCode)
		})
	}
}

func testSupervisorTerminalFenceHoldsCapacityUntilCanonicalCommit(t *testing.T, leaseTTL, advance time.Duration, errorCode string) {
	ctx := context.Background()
	real := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 23, 12, 20, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := kernel.NewPersistentSessionManager(local, real, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
		MissionID: "mission-deadline-capacity", MaxAttempts: 1, Timeout: time.Minute, LeaseTTL: leaseTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "expire while holding capacity", ContextMode: "isolated", Labels: map[string]string{"task_id": "deadline-holder"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: firstID, Attempt: 0, State: kernel.SessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	clock.currentTime = clock.currentTime.Add(advance)

	failing := &rollbackAfterSupervisorUpdateStore{Store: real, fail: true}
	supervisor := &kernel.Supervisor{Store: failing, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if _, err := supervisor.Reconcile(ctx); err == nil {
		t.Fatal("expected injected canonical commit failure")
	}
	status, err := manager.Status(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != kernel.SessionStateFailed || status.Error == nil || status.Error.Error() != errorCode {
		t.Fatalf("process-local generation was not fenced before commit: %+v", status)
	}
	if err := real.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(firstID))
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStatePending {
			t.Fatalf("rolled-back canonical state = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	blockedSpec := kernel.SubagentSpec{Task: "must wait for canonical terminal", ContextMode: "isolated", Labels: map[string]string{"task_id": "deadline-blocked"}}
	if _, err := manager.Spawn(ctx, blockedSpec); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn before terminal commit = %v, want ErrSessionLimit", err)
	}

	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile recovery=(%d,%v)", n, err)
	}
	if err := real.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(firstID))
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStateError || record.ErrorCode != errorCode {
			t.Fatalf("canonical terminal state = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	admittedID, err := manager.Spawn(ctx, blockedSpec)
	if err != nil {
		t.Fatalf("spawn after canonical terminal commit: %v", err)
	}
	if admittedID == "" || admittedID == firstID {
		t.Fatalf("invalid recovered admission id %q", admittedID)
	}
}

func TestSupervisorRetriesTerminalReleaseAfterPostCommitFailure(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 23, 12, 40, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := kernel.NewPersistentSessionManager(local, store, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
		MissionID: "mission-release-retry", MaxAttempts: 1, Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := persistent.Spawn(ctx, kernel.SubagentSpec{Task: "terminal release retry", ContextMode: "isolated", Labels: map[string]string{"task_id": "release-holder"}})
	if err != nil {
		t.Fatal(err)
	}
	clock.currentTime = clock.currentTime.Add(time.Minute)
	manager := &failOnceReleaseSessionManager{SessionManager: persistent, err: errors.New("injected release failure")}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); n != 1 || err == nil || err.Error() != "injected release failure" {
		t.Fatalf("first reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(firstID))
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStateError || record.ErrorCode != "deadline_exceeded" {
			t.Fatalf("committed terminal = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	blocked := kernel.SubagentSpec{Task: "wait for release acknowledgement", ContextMode: "isolated", Labels: map[string]string{"task_id": "release-blocked"}}
	if _, err := persistent.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn while release pending = %v, want ErrSessionLimit", err)
	}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 0 {
		t.Fatalf("release retry reconcile=(%d,%v)", n, err)
	}
	if _, err := persistent.Spawn(ctx, blocked); err != nil {
		t.Fatalf("spawn after retried release: %v", err)
	}
}

func TestSupervisorReleasesExactGenerationAfterTerminalIngressElectionAndPostCommitFailure(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := kernel.NewPersistentSessionManager(local, store, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
		MissionID: "mission-ingress-terminal-release", MaxAttempts: 1, Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := persistent.Spawn(ctx, kernel.SubagentSpec{
		Task:        "elect one terminal transport result",
		ContextMode: "isolated",
		Labels: map[string]string{
			"task_id":                         "ingress-terminal-release-holder",
			kernel.SubagentTransportPeerLabel: "peer-a",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, incoming := range []struct {
		deliveryID  string
		observation kernel.SubagentObservation
	}{
		{deliveryID: "terminal-winner", observation: kernel.SubagentObservation{ID: id, Attempt: 0, State: kernel.SessionStateComplete, Result: "winner"}},
		{deliveryID: "terminal-replay", observation: kernel.SubagentObservation{ID: id, Attempt: 0, State: kernel.SessionStateComplete, Result: "winner"}},
		{deliveryID: "terminal-conflict", observation: kernel.SubagentObservation{ID: id, Attempt: 0, State: kernel.SessionStateFailed, Failure: "divergent"}},
	} {
		if err := persistent.AdmitRemoteStatus(ctx, "peer-a", incoming.deliveryID, incoming.observation); err != nil {
			t.Fatalf("admit %s: %v", incoming.deliveryID, err)
		}
		clock.currentTime = clock.currentTime.Add(time.Second)
	}
	worker := &kernel.SubagentStatusIngressWorker{Store: store, Manager: persistent, Clock: clock, Batch: 3}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 3 {
		t.Fatalf("terminal ingress election=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		winner, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-winner")
		if err != nil {
			return err
		}
		replay, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-replay")
		if err != nil {
			return err
		}
		conflict, err := r.SubagentStatusIngressReceipt("peer-a", "terminal-conflict")
		if err != nil {
			return err
		}
		if winner.Status != domain.SubagentStatusIngressApplied || replay.Status != domain.SubagentStatusIngressApplied || conflict.Status != domain.SubagentStatusIngressRejected || conflict.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Fatalf("winner=%+v replay=%+v conflict=%+v", winner, replay, conflict)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	blocked := kernel.SubagentSpec{Task: "wait for elected terminal durability", ContextMode: "isolated", Labels: map[string]string{"task_id": "ingress-terminal-release-blocked"}}
	if _, err := persistent.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn before canonical terminal = %v, want ErrSessionLimit", err)
	}
	manager := &failOnceReleaseSessionManager{SessionManager: persistent, err: errors.New("injected post-commit release failure")}
	supervisor := &kernel.Supervisor{Store: store, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := supervisor.Reconcile(ctx); n != 1 || err == nil || err.Error() != "injected post-commit release failure" {
		t.Fatalf("first reconcile=(%d,%v)", n, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(id))
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStateComplete || record.Attempt != 0 || record.Result != "winner" {
			t.Fatalf("committed elected terminal = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := persistent.ReleaseTerminal(ctx, id, 1); !errors.Is(err, kernel.ErrSessionAttempt) {
		t.Fatalf("wrong-generation release = %v, want ErrSessionAttempt", err)
	}
	if _, err := persistent.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn after wrong-generation release = %v, want ErrSessionLimit", err)
	}

	restartedSupervisor := &kernel.Supervisor{Store: store, Manager: persistent, Clock: clock, IDs: &supervisorIDs{}}
	if n, err := restartedSupervisor.Reconcile(ctx); err != nil || n != 0 {
		t.Fatalf("release recovery reconcile=(%d,%v)", n, err)
	}
	if _, err := persistent.Spawn(ctx, blocked); err != nil {
		t.Fatalf("spawn after exact-generation release recovery: %v", err)
	}
}

func TestSupervisorTransportTerminalHoldsCapacityUntilCanonicalCommit(t *testing.T) {
	for _, tc := range []struct {
		name        string
		observation kernel.SubagentObservation
		wantState   domain.SubagentState
		wantResult  string
		wantError   string
	}{
		{
			name:        "complete",
			observation: kernel.SubagentObservation{State: kernel.SessionStateComplete, Result: "transport result"},
			wantState:   domain.SubagentStateComplete,
			wantResult:  "transport result",
		},
		{
			name:        "failed",
			observation: kernel.SubagentObservation{State: kernel.SessionStateFailed, Failure: "transport_failed"},
			wantState:   domain.SubagentStateError,
			wantError:   "transport_failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			real := memory.New()
			clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 23, 13, 20, 0, 0, time.UTC)}
			local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
			if err != nil {
				t.Fatal(err)
			}
			manager, err := kernel.NewPersistentSessionManager(local, real, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
				MissionID: "mission-transport-terminal-capacity", MaxAttempts: 1, Timeout: time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			id, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "transport terminal before commit", ContextMode: "isolated", Labels: map[string]string{"task_id": "transport-terminal-" + tc.name}})
			if err != nil {
				t.Fatal(err)
			}
			observation := tc.observation
			observation.ID = id
			observation.Attempt = 0
			if err := manager.PublishStatus(ctx, observation); err != nil {
				t.Fatal(err)
			}

			blocked := kernel.SubagentSpec{Task: "wait for transport terminal commit", ContextMode: "isolated", Labels: map[string]string{"task_id": "transport-terminal-blocked-" + tc.name}}
			if _, err := manager.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
				t.Fatalf("spawn before canonical commit = %v, want ErrSessionLimit", err)
			}
			failing := &rollbackAfterSupervisorUpdateStore{Store: real, fail: true}
			supervisor := &kernel.Supervisor{Store: failing, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
			if _, err := supervisor.Reconcile(ctx); err == nil {
				t.Fatal("expected injected transport-terminal commit failure")
			}
			if _, err := manager.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
				t.Fatalf("spawn after rolled-back terminal commit = %v, want ErrSessionLimit", err)
			}
			if err := real.View(ctx, func(r port.Reader) error {
				record, err := r.SubagentRecord(string(id))
				if err != nil {
					return err
				}
				if record.State != domain.SubagentStatePending {
					t.Fatalf("rolled-back canonical transport terminal = %+v", record)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
				t.Fatalf("transport terminal recovery=(%d,%v)", n, err)
			}
			if err := real.View(ctx, func(r port.Reader) error {
				record, err := r.SubagentRecord(string(id))
				if err != nil {
					return err
				}
				if record.State != tc.wantState || record.Result != tc.wantResult || record.ErrorCode != tc.wantError {
					t.Fatalf("canonical transport terminal = %+v", record)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Spawn(ctx, blocked); err != nil {
				t.Fatalf("spawn after transport terminal commit: %v", err)
			}
		})
	}
}

func TestSupervisorRecoveredRetryExpiryHoldsCapacityUntilCanonicalCommit(t *testing.T) {
	ctx := context.Background()
	real := memory.New()
	clock := &supervisorMockClock{currentTime: time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)}
	local, err := kernel.NewLocalSessionManagerWithPolicy(clock, kernel.SessionPolicy{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := kernel.NewPersistentSessionManager(local, real, clock, &supervisorIDs{}, kernel.PersistentSessionPolicy{
		MissionID: "mission-recovered-retry-capacity", MaxAttempts: 2, Timeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(ctx, kernel.SubagentSpec{Task: "retry across rollback", ContextMode: "isolated", Labels: map[string]string{"task_id": "recovered-retry-holder"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: id, Attempt: 0, State: kernel.SessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	supervisor := &kernel.Supervisor{Store: real, Manager: manager, Clock: clock, IDs: &supervisorIDs{}}
	if _, err := supervisor.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := manager.PublishStatus(ctx, kernel.SubagentObservation{ID: id, Attempt: 0, State: kernel.SessionStateFailed, Failure: "transient"}); err != nil {
		t.Fatal(err)
	}
	failing := &rollbackAfterSupervisorUpdateStore{Store: real, fail: true}
	supervisor.Store = failing
	if _, err := supervisor.Reconcile(ctx); err == nil {
		t.Fatal("expected retry publication rollback")
	}
	status, err := manager.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Attempt != 1 || status.State != kernel.SessionStatePending {
		t.Fatalf("recovered retry generation = %+v", status)
	}
	clock.currentTime = clock.currentTime.Add(time.Minute)
	failing.fail = true
	if _, err := supervisor.Reconcile(ctx); err == nil {
		t.Fatal("expected recovered-retry terminal rollback")
	}
	status, err = manager.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Attempt != 1 || status.State != kernel.SessionStateFailed || status.Error == nil || status.Error.Error() != "deadline_exceeded" {
		t.Fatalf("recovered retry was not fenced: %+v", status)
	}
	blocked := kernel.SubagentSpec{Task: "wait for recovered retry commit", ContextMode: "isolated", Labels: map[string]string{"task_id": "recovered-retry-blocked"}}
	if _, err := manager.Spawn(ctx, blocked); !errors.Is(err, kernel.ErrSessionLimit) {
		t.Fatalf("spawn before recovered retry commit = %v, want ErrSessionLimit", err)
	}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("recovered retry commit=(%d,%v)", n, err)
	}
	if err := real.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(id))
		if err != nil {
			return err
		}
		if record.Attempt != 1 || record.State != domain.SubagentStateError || record.ErrorCode != "deadline_exceeded" {
			t.Fatalf("recovered retry canonical terminal = %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Spawn(ctx, blocked); err != nil {
		t.Fatalf("spawn after recovered retry commit: %v", err)
	}
}
