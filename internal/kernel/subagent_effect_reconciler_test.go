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
		if got.Status != domain.SubagentDispatchEffectUnknown || got.ReconcileAttempts != 1 || !got.ReconcileAfter.Equal(clock.Now().Add(time.Second)) {
			t.Fatalf("dispatch=%+v", got)
		}
		return nil
	})
}

func TestSubagentEffectReconcilerBackoffPreventsSameKindStarvation(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	first := seedDispatch(t, store, clock.Now())
	leased, _ := domain.LeaseSubagentDispatch(first, "worker", clock.Now(), clock.Now().Add(time.Second))
	firstUnknown, _ := domain.MarkAmbiguousSubagentDispatch(leased, "worker", clock.Now())
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(firstUnknown, first.Status, first.SendAttempt)
	}); err != nil {
		t.Fatal(err)
	}

	clock.currentTime = clock.Now().Add(time.Millisecond)
	second := seedDispatchWithID(t, store, clock.Now(), "dispatch-2", "session-2")
	leased, _ = domain.LeaseSubagentDispatch(second, "worker", clock.Now(), clock.Now().Add(time.Second))
	secondUnknown, _ := domain.MarkAmbiguousSubagentDispatch(leased, "worker", clock.Now())
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(secondUnknown, second.Status, second.SendAttempt)
	}); err != nil {
		t.Fatal(err)
	}

	lookups := make([]string, 0, 2)
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		lookups = append(lookups, lookup.DeliveryID)
		state := domain.SubagentReconcileNotFound
		receiver := ""
		if lookup.DeliveryID == string(second.RequestID) {
			state, receiver = domain.SubagentReconcileFound, "remote-2"
		}
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: state, ReceiverSessionID: receiver})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock, Batch: 1, BackoffBase: time.Minute}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 {
		t.Fatalf("first reconcile=(%d,%v)", n, err)
	}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 1 {
		t.Fatalf("second reconcile=(%d,%v)", n, err)
	}
	if len(lookups) != 2 || lookups[0] != string(first.RequestID) || lookups[1] != string(second.RequestID) {
		t.Fatalf("lookups=%v", lookups)
	}
}

func TestSubagentEffectReconcilerWaitsForDurableBackoff(t *testing.T) {
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
	lookups := 0
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		lookups++
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: domain.SubagentReconcileNotFound})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock, BackoffBase: time.Second}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 || lookups != 1 {
		t.Fatalf("first reconcile=(%d,%v) lookups=%d", n, err, lookups)
	}
	clock.currentTime = clock.Now().Add(time.Second - time.Nanosecond)
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 || lookups != 1 {
		t.Fatalf("early reconcile=(%d,%v) lookups=%d", n, err, lookups)
	}
	clock.currentTime = clock.Now().Add(time.Nanosecond)
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 || lookups != 2 {
		t.Fatalf("due reconcile=(%d,%v) lookups=%d", n, err, lookups)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if got.ReconcileAttempts != 2 || !got.ReconcileAfter.Equal(clock.Now().Add(2*time.Second)) {
			t.Fatalf("dispatch=%+v", got)
		}
		return nil
	})
}

func TestSubagentEffectReconcilerBackoffStartsAfterSlowLookup(t *testing.T) {
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
		clock.currentTime = clock.Now().Add(5 * time.Second)
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: domain.SubagentReconcileNotFound})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock, BackoffBase: time.Second}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if !got.UpdatedAt.Equal(clock.Now()) || !got.ReconcileAfter.Equal(clock.Now().Add(time.Second)) {
			t.Fatalf("dispatch=%+v now=%s", got, clock.Now())
		}
		return nil
	})
}

func TestSubagentEffectReconcilerDoesNotDeferConcurrentStatusUpdate(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	receipt := seedTerminalReceipt(t, store, clock.Now())
	inFlight, err := domain.BeginSubagentSpawnReceiptStatusDelivery(receipt, clock.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := domain.MarkSubagentSpawnReceiptStatusEffectUnknown(inFlight, clock.Now().Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(unknown, receipt.Status, receipt.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	clock.currentTime = unknown.UpdatedAt
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			current, err := tx.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
			if err != nil {
				return err
			}
			next, err := domain.DeferSubagentSpawnReceiptStatusReconciliation(current, clock.Now(), clock.Now().Add(30*time.Second))
			if err != nil {
				return err
			}
			return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
		}); err != nil {
			t.Fatal(err)
		}
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: domain.SubagentReconcileNotFound})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock, BackoffBase: time.Second}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 0 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if got.ReconcileAttempts != 1 || !got.ReconcileAfter.Equal(clock.Now().Add(30*time.Second)) {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	})
}

func TestSubagentEffectReconcilerUsesOldestEvidenceAcrossKinds(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	receipt := seedTerminalReceipt(t, store, clock.Now())
	inFlight, err := domain.BeginSubagentSpawnReceiptStatusDelivery(receipt, clock.Now().Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	unknownStatus, err := domain.MarkSubagentSpawnReceiptStatusEffectUnknown(inFlight, clock.Now().Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentSpawnReceipt(unknownStatus, receipt.Status, receipt.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}

	clock.currentTime = clock.Now().Add(10 * time.Second)
	dispatch := seedDispatch(t, store, clock.Now())
	leased, _ := domain.LeaseSubagentDispatch(dispatch, "worker", clock.Now(), clock.Now().Add(time.Second))
	unknownDispatch, _ := domain.MarkAmbiguousSubagentDispatch(leased, "worker", clock.Now())
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveSubagentDispatch(unknownDispatch, dispatch.Status, dispatch.SendAttempt)
	}); err != nil {
		t.Fatal(err)
	}

	var lookedUp domain.SubagentReconcileKind
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		lookup, _ := domain.DecodeSubagentReconcileRequest(request.Payload)
		lookedUp = lookup.Kind
		payload, _ := domain.EncodeSubagentReconcileResponse(domain.SubagentReconcileResponse{Kind: lookup.Kind, DeliveryID: lookup.DeliveryID, SessionID: lookup.SessionID, Attempt: lookup.Attempt, State: domain.SubagentReconcileFound})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	reconciler := kernel.SubagentEffectReconciler{Store: store, Caller: caller, Clock: clock, Batch: 1}
	if n, err := reconciler.Reconcile(context.Background()); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	if lookedUp != domain.SubagentReconcileStatus {
		t.Fatalf("looked up %q, want oldest STATUS", lookedUp)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		gotStatus, _ := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		gotDispatch, _ := r.SubagentDispatch(dispatch.RequestID)
		if gotStatus.StatusDelivery != domain.SubagentStatusDeliveryDelivered || gotDispatch.Status != domain.SubagentDispatchEffectUnknown {
			t.Fatalf("status=%s dispatch=%s", gotStatus.StatusDelivery, gotDispatch.Status)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
