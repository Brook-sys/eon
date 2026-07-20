package kernel_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

type dispatchCaller func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)

func (f dispatchCaller) Call(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return f(ctx, request)
}

func seedDispatch(t *testing.T, store port.Store, now time.Time) domain.SubagentDispatch {
	return seedDispatchWithID(t, store, now, "dispatch-1", "session-1")
}

func seedDispatchWithID(t *testing.T, store port.Store, now time.Time, requestID, sessionID string) domain.SubagentDispatch {
	t.Helper()
	record := domain.SubagentRecord{SchemaVersion: domain.SchemaVersionV1, ID: sessionID, TaskID: "task-" + sessionID, MissionID: "mission-1", State: domain.SubagentStatePending, StartedAt: now, UpdatedAt: now, Task: "remote task", ContextMode: "isolated", TransportPeerID: "peer-a", MaxAttempts: 2, Deadline: now.Add(time.Minute)}
	dispatch := domain.SubagentDispatch{SchemaVersion: domain.SchemaVersionV1, RequestID: domain.SubagentDispatchRequestID(requestID), SessionID: record.ID, PeerID: "peer-a", Status: domain.SubagentDispatchPending, MaxSendAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateSubagentRecord(record); err != nil {
			return err
		}
		return tx.CreateSubagentDispatch(dispatch)
	}); err != nil {
		t.Fatal(err)
	}
	return dispatch
}

func TestSubagentDispatcherDeliversCorrelatedAcknowledgement(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	dispatch := seedDispatch(t, store, clock.Now())
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		spawn, err := domain.DecodeSubagentSpawnRequest(request.Payload)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(domain.SubagentSpawnAcknowledgement{RequestID: spawn.RequestID, SessionID: spawn.SessionID, Attempt: spawn.Attempt, ReceiverSessionID: "remote-1", Accepted: true})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	worker := kernel.SubagentDispatcher{Store: store, Caller: caller, Clock: clock, Owner: "worker-1"}
	if n, err := worker.DispatchDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("dispatch=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if got.Status != domain.SubagentDispatchDelivered || got.SendAttempt != 1 || got.ReceiverSessionID != "remote-1" {
			t.Fatalf("dispatch = %+v", got)
		}
		return nil
	})
}

func TestSubagentDispatcherLeavesTimeoutEffectUnknown(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	dispatch := seedDispatch(t, store, clock.Now())
	caller := dispatchCaller(func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		return domain.PeerRPCResponse{}, context.DeadlineExceeded
	})
	worker := kernel.SubagentDispatcher{Store: store, Caller: caller, Clock: clock, Owner: "worker-1"}
	if n, err := worker.DispatchDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("dispatch=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if got.Status != domain.SubagentDispatchEffectUnknown {
			t.Fatalf("dispatch = %+v", got)
		}
		return nil
	})
	if n, err := worker.DispatchDue(context.Background()); err != nil || n != 0 {
		t.Fatalf("effect unknown replay=(%d,%v)", n, err)
	}
}

func TestSubagentDispatcherRetriesDefiniteFailure(t *testing.T) {
	store := memory.New()
	clock := &supervisorMockClock{currentTime: time.Unix(100, 0).UTC()}
	dispatch := seedDispatch(t, store, clock.Now())
	caller := dispatchCaller(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		payload, _ := json.Marshal(domain.SubagentSpawnAcknowledgement{RequestID: request.RequestID, SessionID: "session-1", Code: "BUSY", Retryable: true})
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	worker := kernel.SubagentDispatcher{Store: store, Caller: caller, Clock: clock, Owner: "worker-1", RetryDelay: time.Minute}
	if n, err := worker.DispatchDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("dispatch=(%d,%v)", n, err)
	}
	_ = store.View(context.Background(), func(r port.Reader) error {
		got, _ := r.SubagentDispatch(dispatch.RequestID)
		if got.Status != domain.SubagentDispatchRetry || !got.AvailableAt.Equal(clock.Now().Add(time.Minute)) {
			t.Fatalf("dispatch = %+v", got)
		}
		return nil
	})
}
