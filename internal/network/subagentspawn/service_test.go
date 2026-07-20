package subagentspawn

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(100, 0).UTC() }

type fixedIDs struct{ next int }

func (i *fixedIDs) NewID(prefix string) (string, error) {
	i.next++
	return prefix + "-id", nil
}

func persistentManager(t *testing.T, store *memory.Store) *kernel.PersistentSessionManager {
	t.Helper()
	local := kernel.NewLocalSessionManager(fixedClock{})
	manager, err := kernel.NewPersistentSessionManager(local, store, fixedClock{}, &fixedIDs{}, kernel.PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestServiceAdmitsAuthenticatedReplayExactlyOnce(t *testing.T) {
	manager := persistentManager(t, memory.New())
	service, err := NewService(manager)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeRequest(Request{RequestID: "dispatch-1", SessionID: "source-1", Attempt: 2, Task: "inspect evidence", ContextMode: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	ack1, err := DecodeAcknowledgement(first)
	if err != nil {
		t.Fatal(err)
	}
	ack2, err := DecodeAcknowledgement(second)
	if err != nil {
		t.Fatal(err)
	}
	if ack1 != ack2 || ack1.RequestID != "dispatch-1" || ack1.Attempt != 2 {
		t.Fatalf("acks = %+v %+v", ack1, ack2)
	}
}

func TestServiceRejectsMalformedAndConflictingReplay(t *testing.T) {
	manager := persistentManager(t, memory.New())
	service, _ := NewService(manager)
	payload, _ := EncodeRequest(Request{RequestID: "dispatch-1", SessionID: "source-1", Task: "first", ContextMode: "isolated"})
	if _, err := service.Handle(context.Background(), "peer-a", payload); err != nil {
		t.Fatal(err)
	}
	conflict, _ := EncodeRequest(Request{RequestID: "dispatch-1", SessionID: "source-1", Task: "different", ContextMode: "isolated"})
	if _, err := service.Handle(context.Background(), "peer-a", conflict); err != kernel.ErrSessionConflict {
		t.Fatalf("conflict error = %v", err)
	}
	if _, err := service.Handle(context.Background(), "", payload); err != ErrInvalidRequest {
		t.Fatalf("caller error = %v", err)
	}
	if _, err := service.Handle(context.Background(), "peer-a", []byte(`{"request_id":"x","extra":true}`)); err != ErrInvalidRequest {
		t.Fatalf("malformed error = %v", err)
	}
}

func TestServiceScopesRequestIdentityByAuthenticatedPeer(t *testing.T) {
	manager := persistentManager(t, memory.New())
	service, _ := NewService(manager)
	payload, _ := EncodeRequest(Request{RequestID: "shared-request", SessionID: "source-1", Task: "first", ContextMode: "isolated"})
	peerA, err := service.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := service.Handle(context.Background(), "peer-b", payload)
	if err != nil {
		t.Fatal(err)
	}
	ackA, _ := DecodeAcknowledgement(peerA)
	ackB, _ := DecodeAcknowledgement(peerB)
	if ackA.ReceiverSessionID == ackB.ReceiverSessionID {
		t.Fatalf("distinct authenticated peers shared receiver session %q", ackA.ReceiverSessionID)
	}
}

func TestServiceReplaysDurableReceiptAfterRestart(t *testing.T) {
	store := memory.New()
	payload, err := EncodeRequest(Request{RequestID: "dispatch-restart", SessionID: "source-1", Attempt: 1, Task: "inspect evidence", ContextMode: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	firstService, _ := NewService(persistentManager(t, store))
	firstPayload, err := firstService.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restartedStore, err := memory.NewFromBinary(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	restartedService, _ := NewService(persistentManager(t, restartedStore))
	secondPayload, err := restartedService.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := DecodeAcknowledgement(firstPayload)
	second, _ := DecodeAcknowledgement(secondPayload)
	if first != second || first.ReceiverSessionID == "" {
		t.Fatalf("restart replay acknowledgements differ: %+v %+v", first, second)
	}
	conflict, _ := EncodeRequest(Request{RequestID: "dispatch-restart", SessionID: "source-1", Attempt: 1, Task: "changed task", ContextMode: "isolated"})
	if _, err := restartedService.Handle(context.Background(), "peer-a", conflict); err != kernel.ErrSessionConflict {
		t.Fatalf("conflicting restart replay error = %v", err)
	}
	if err := restartedStore.View(context.Background(), func(r port.Reader) error {
		receipt, readErr := r.SubagentSpawnReceipt("peer-a", "dispatch-restart")
		if readErr != nil {
			return readErr
		}
		if receipt.ReceiverSessionID != first.ReceiverSessionID || !receipt.Matches("peer-a", Request{RequestID: "dispatch-restart", SessionID: "source-1", Attempt: 1, Task: "inspect evidence", ContextMode: "isolated"}) {
			t.Fatalf("unexpected receipt: %+v", receipt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
