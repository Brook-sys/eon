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

func TestSubagentEffectReconcilerCompletesOnlyPositiveSpawnEvidence(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	dispatch := seedDispatch(t, store, clock.Now())
	leased, _ := domain.LeaseSubagentDispatch(dispatch, "worker", clock.Now(), clock.Now().Add(time.Second))
	unknown, _ := domain.MarkAmbiguousSubagentDispatch(leased, "worker", clock.Now())
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(unknown, dispatch.Status, dispatch.SendAttempt)
	}); err != nil {
		t.Fatal(err)
	}
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: domain.SubagentReconcileFound, ReceiverSessionID: "remote-1"})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if got.Status != domain.SubagentDispatchDelivered || got.ReceiverSessionID != "remote-1" {
			t.Fatalf("dispatch=%+v", got)
		}
		return nil
	})
}

func TestSubagentEffectReconcilerLeavesAbsentSpawnParked(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	dispatch := seedDispatch(t, store, clock.Now())
	leased, _ := domain.LeaseSubagentDispatch(dispatch, "worker", clock.Now(), clock.Now().Add(time.Second))
	unknown, _ := domain.MarkAmbiguousSubagentDispatch(leased, "worker", clock.Now())
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(unknown, dispatch.Status, dispatch.SendAttempt)
	}); err != nil {
		t.Fatal(err)
	}
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: domain.SubagentReconcileNotFound})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if got.Status != domain.SubagentDispatchEffectUnknown {
			t.Fatalf("dispatch=%+v", got)
		}
		return nil
	})
}
