package subagentstatus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fixedIDs struct{ n int }

func (i *fixedIDs) NewID(prefix string) (string, error) { i.n++; return prefix + "-id", nil }

func statusFixture(t *testing.T) (*kernel.PersistentSessionManager, *memory.Store, kernel.SessionID) {
	t.Helper()
	clock := fixedClock{now: time.Unix(100, 0).UTC()}
	store := memory.New()
	local := kernel.NewLocalSessionManager(clock)
	manager, err := kernel.NewPersistentSessionManager(local, store, clock, &fixedIDs{}, kernel.PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "inspect evidence", ContextMode: "isolated", Labels: map[string]string{TransportPeerKey: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	return manager, store, id
}

func TestServiceDurablyAdmitsAuthenticatedObservationBeforeACK(t *testing.T) {
	manager, store, id := statusFixture(t)
	service, _ := NewService(manager)
	payload, _ := Encode(Observation{DeliveryID: "dispatch-1", SessionID: string(id), State: kernel.SessionStateComplete, Result: "bounded evidence"})
	response, err := service.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := DecodeAcknowledgement(response)
	if err != nil || ack.SessionID != string(id) || ack.State != kernel.SessionStateComplete {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	status, _ := manager.Status(context.Background(), id)
	if status.State != kernel.SessionStatePending {
		t.Fatalf("network ingress applied process-local status: %+v", status)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		receipt, err := r.SubagentStatusIngressReceipt("peer-a", "dispatch-1")
		if err != nil {
			return err
		}
		if receipt.Status != domain.SubagentStatusIngressPending || receipt.Result != "bounded evidence" {
			t.Fatalf("receipt=%+v", receipt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(context.Background(), "peer-a", payload); err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	divergent, _ := Encode(Observation{DeliveryID: "dispatch-1", SessionID: string(id), State: kernel.SessionStateComplete, Result: "different"})
	if _, err := service.Handle(context.Background(), "peer-a", divergent); !errors.Is(err, kernel.ErrSessionConflict) {
		t.Fatalf("divergent replay=%v", err)
	}
}

func TestServiceRejectsWrongPeerMalformedAndOversize(t *testing.T) {
	manager, _, id := statusFixture(t)
	service, _ := NewService(manager)
	payload, _ := Encode(Observation{DeliveryID: "dispatch-1", SessionID: string(id), State: kernel.SessionStateComplete, Result: "done"})
	if _, err := service.Handle(context.Background(), "peer-b", payload); !errors.Is(err, domain.ErrInvalidSubagentStatusIngress) {
		t.Fatalf("wrong peer=%v", err)
	}
	for _, bad := range [][]byte{[]byte(`{"delivery_id":"d","session_id":"s","state":"RUNNING","extra":true}`), []byte(`{} trailing`)} {
		if _, err := service.Handle(context.Background(), "peer-a", bad); !errors.Is(err, ErrInvalidObservation) {
			t.Fatalf("malformed=%v", err)
		}
	}
	if payload, _ := Encode(Observation{DeliveryID: "d", SessionID: string(id), State: kernel.SessionStateComplete, Result: strings.Repeat("x", maxResultBytes+1)}); payload != nil {
		t.Fatal("oversize encoded")
	}
}
