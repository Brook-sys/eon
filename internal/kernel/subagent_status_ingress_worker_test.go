package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

type ingressWorkerClock struct{ now time.Time }

func (c ingressWorkerClock) Now() time.Time { return c.now }

type ingressWorkerIDs struct{ n int }

func (i *ingressWorkerIDs) NewID(prefix string) (string, error) { i.n++; return prefix + "-id", nil }

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
