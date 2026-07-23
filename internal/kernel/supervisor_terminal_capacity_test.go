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
