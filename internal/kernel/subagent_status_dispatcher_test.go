package kernel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/network/subagentstatus"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func seedTerminalReceipt(t *testing.T, store port.Store, now time.Time) domain.SubagentSpawnReceipt {
	t.Helper()
	record := domain.SubagentRecord{SchemaVersion: 1, ID: "receiver-1", TaskID: "receiver-task", MissionID: "mission-1", State: domain.SubagentStateComplete, StartedAt: now, UpdatedAt: now.Add(time.Second), Task: "work", ContextMode: "isolated", Result: "answer", MaxAttempts: 3}
	receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-origin", RequestID: "request-1", SourceSessionID: "source-1", Attempt: 3, Task: "work", ContextMode: "isolated", ReceiverSessionID: record.ID, RecordedAt: now, Status: domain.SubagentSpawnReceiptComplete, UpdatedAt: now.Add(time.Second), Result: "answer"}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentSpawnReceipt(receipt)
	}); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestSubagentStatusDispatcherUsesSourceGenerationAndMarksACK(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	receipt := seedTerminalReceipt(t, store, clock.Now())
	clock.currentTime = clock.Now().Add(2 * time.Second)
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		if request.PeerID != receipt.CallerPeerID || request.Capability != "subagent.status.v1" {
			t.Fatalf("route=%+v", request)
		}
		var observation struct {
			SessionID string              `json:"session_id"`
			Attempt   int                 `json:"attempt"`
			State     kernel.SessionState `json:"state"`
			Result    string              `json:"result"`
		}
		if err := json.Unmarshal(request.Payload, &observation); err != nil {
			t.Fatal(err)
		}
		if observation.SessionID != receipt.SourceSessionID || observation.Attempt != receipt.Attempt || observation.State != kernel.SessionStateComplete || observation.Result != "answer" {
			t.Fatalf("observation=%+v", observation)
		}
		payload, _ := json.Marshal(map[string]any{"session_id": observation.SessionID, "state": observation.State})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	dispatcher := kernel.SubagentStatusDispatcher{Store: store, Caller: caller, Clock: clock}
	if n, err := dispatcher.DispatchTerminal(context.Background()); err != nil || n != 1 {
		t.Fatalf("dispatch=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if got.StatusDelivery != domain.SubagentStatusDeliveryDelivered || got.Result != receipt.Result {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	})
	if n, err := dispatcher.DispatchTerminal(context.Background()); err != nil || n != 0 {
		t.Fatalf("replay=(%d,%v)", n, err)
	}
}

func TestSubagentStatusDispatcherRetainsTerminalEvidenceAfterCallFailure(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	receipt := seedTerminalReceipt(t, store, clock.Now())
	clock.currentTime = clock.Now().Add(2 * time.Second)
	dispatcher := kernel.SubagentStatusDispatcher{Store: store, Clock: clock, Caller: dispatchCaller(func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		return domain.PeerRPCResponse{}, errors.New("transport unavailable")
	})}
	if n, err := dispatcher.DispatchTerminal(context.Background()); err != nil || n != 1 {
		t.Fatalf("dispatch=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
		if got.StatusDelivery != domain.SubagentStatusDeliveryEffectUnknown || got.Status != domain.SubagentSpawnReceiptComplete || got.Result != "answer" {
			t.Fatalf("receipt=%+v", got)
		}
		return nil
	})
}

func TestSubagentStatusDispatcherCompletesOriginGenerationThroughSupervisor(t *testing.T) {
	ctx := context.Background()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	receiverStore := memory.New()
	receipt := seedTerminalReceipt(t, receiverStore, clock.Now())

	originStore := memory.New()
	originManager := kernel.NewLocalSessionManager(clock)
	if err := originManager.Restore(ctx, kernel.SubagentStatus{
		ID:      kernel.SessionID(receipt.SourceSessionID),
		Attempt: receipt.Attempt,
		State:   kernel.SessionStatePending,
		Spec: kernel.SubagentSpec{
			Task:        receipt.Task,
			ContextMode: receipt.ContextMode,
			Labels:      map[string]string{kernel.SubagentTransportPeerLabel: "peer-receiver"},
		},
		StartedAt: clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	originRecord := domain.SubagentRecord{SchemaVersion: 1, ID: receipt.SourceSessionID, TaskID: "origin-task", MissionID: "mission-origin", State: domain.SubagentStatePending, StartedAt: clock.Now(), UpdatedAt: clock.Now(), Task: receipt.Task, ContextMode: receipt.ContextMode, TransportPeerID: "peer-receiver", Attempt: receipt.Attempt, MaxAttempts: 4}
	if err := originStore.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentRecord(originRecord) }); err != nil {
		t.Fatal(err)
	}
	service, err := subagentstatus.NewService(originManager)
	if err != nil {
		t.Fatal(err)
	}
	caller := dispatchCaller(func(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		if request.PeerID != receipt.CallerPeerID {
			t.Fatalf("status route peer=%q", request.PeerID)
		}
		payload, err := service.Handle(ctx, "peer-receiver", request.Payload)
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, err
	})
	clock.currentTime = clock.Now().Add(2 * time.Second)
	dispatcher := kernel.SubagentStatusDispatcher{Store: receiverStore, Caller: caller, Clock: clock}
	if n, err := dispatcher.DispatchTerminal(ctx); err != nil || n != 1 {
		t.Fatalf("dispatch=(%d,%v)", n, err)
	}
	supervisor := kernel.Supervisor{Store: originStore, Manager: originManager, Clock: clock}
	if n, err := supervisor.Reconcile(ctx); err != nil || n != 1 {
		t.Fatalf("reconcile=(%d,%v)", n, err)
	}
	if err := originStore.View(ctx, func(r port.Reader) error {
		got, err := r.SubagentRecord(receipt.SourceSessionID)
		if err != nil {
			return err
		}
		if got.State != domain.SubagentStateComplete || got.Attempt != receipt.Attempt || got.Result != receipt.Result {
			t.Fatalf("origin record=%+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
