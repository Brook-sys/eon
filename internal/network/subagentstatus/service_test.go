package subagentstatus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestServicePublishesAuthenticatedBoundedObservation(t *testing.T) {
	manager := kernel.NewLocalSessionManager(fixedClock{now: time.Unix(100, 0).UTC()})
	id, err := manager.Spawn(context.Background(), kernel.SubagentSpec{
		Task: "inspect evidence", ContextMode: "isolated", Labels: map[string]string{TransportPeerKey: "peer-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(manager)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Encode(Observation{SessionID: string(id), State: kernel.SessionStateComplete, Result: "bounded evidence"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.Handle(context.Background(), "peer-a", payload)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := DecodeAcknowledgement(response)
	if err != nil || ack.SessionID != string(id) || ack.State != kernel.SessionStateComplete {
		t.Fatalf("ack = %+v, err=%v", ack, err)
	}
	status, err := manager.Status(context.Background(), id)
	if err != nil || status.State != kernel.SessionStateComplete || status.Result != "bounded evidence" {
		t.Fatalf("status = %+v, err=%v", status, err)
	}
	// Exact terminal replay is harmless at the authenticated ingress.
	if _, err := service.Handle(context.Background(), "peer-a", payload); err != nil {
		t.Fatalf("terminal replay: %v", err)
	}
}

func TestServiceRejectsWrongPeerAndUnboundSession(t *testing.T) {
	manager := kernel.NewLocalSessionManager(fixedClock{now: time.Unix(100, 0).UTC()})
	bound, _ := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "bound", ContextMode: "isolated", Labels: map[string]string{TransportPeerKey: "peer-a"}})
	unbound, _ := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "unbound", ContextMode: "isolated"})
	service, _ := NewService(manager)

	for _, tc := range []struct {
		caller string
		id     kernel.SessionID
	}{
		{caller: "peer-b", id: bound},
		{caller: "peer-a", id: unbound},
	} {
		payload, _ := Encode(Observation{SessionID: string(tc.id), State: kernel.SessionStateRunning})
		if _, err := service.Handle(context.Background(), tc.caller, payload); !errors.Is(err, ErrInvalidObservation) {
			t.Fatalf("caller=%q id=%q error=%v", tc.caller, tc.id, err)
		}
		status, _ := manager.Status(context.Background(), tc.id)
		if status.State != kernel.SessionStatePending {
			t.Fatalf("rejected publication changed state to %s", status.State)
		}
	}
}

func TestServiceRejectsMalformedOversizeAndDivergentTerminal(t *testing.T) {
	manager := kernel.NewLocalSessionManager(fixedClock{now: time.Unix(100, 0).UTC()})
	id, _ := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "task", ContextMode: "isolated", Labels: map[string]string{TransportPeerKey: "peer-a"}})
	service, _ := NewService(manager)

	for _, payload := range [][]byte{
		[]byte(`{"session_id":"` + string(id) + `","state":"RUNNING","extra":true}`),
		[]byte(`{"session_id":"` + string(id) + `","state":"RUNNING"} trailing`),
	} {
		if _, err := service.Handle(context.Background(), "peer-a", payload); !errors.Is(err, ErrInvalidObservation) {
			t.Fatalf("malformed error = %v", err)
		}
	}
	oversize, _ := Encode(Observation{SessionID: string(id), State: kernel.SessionStateComplete, Result: strings.Repeat("x", maxResultBytes+1)})
	if oversize != nil {
		t.Fatal("oversize observation unexpectedly encoded")
	}

	first, _ := Encode(Observation{SessionID: string(id), State: kernel.SessionStateComplete, Result: "first"})
	if _, err := service.Handle(context.Background(), "peer-a", first); err != nil {
		t.Fatal(err)
	}
	divergent, _ := Encode(Observation{SessionID: string(id), State: kernel.SessionStateComplete, Result: "other"})
	if _, err := service.Handle(context.Background(), "peer-a", divergent); !errors.Is(err, kernel.ErrSessionTerminal) {
		t.Fatalf("divergent terminal error = %v", err)
	}
}

func TestServiceRejectsStaleAttemptAfterRetry(t *testing.T) {
	manager := kernel.NewLocalSessionManager(fixedClock{now: time.Unix(100, 0).UTC()})
	id, _ := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "task", ContextMode: "isolated", Labels: map[string]string{TransportPeerKey: "peer-a"}})
	service, _ := NewService(manager)
	failed, _ := Encode(Observation{SessionID: string(id), Attempt: 0, State: kernel.SessionStateFailed, Failure: "temporary"})
	if _, err := service.Handle(context.Background(), "peer-a", failed); err != nil {
		t.Fatal(err)
	}
	if err := manager.Retry(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	stale, _ := Encode(Observation{SessionID: string(id), Attempt: 0, State: kernel.SessionStateComplete, Result: "late"})
	if _, err := service.Handle(context.Background(), "peer-a", stale); !errors.Is(err, kernel.ErrSessionAttempt) {
		t.Fatalf("stale attempt error = %v", err)
	}
	status, _ := manager.Status(context.Background(), id)
	if status.State != kernel.SessionStatePending || status.Attempt != 1 {
		t.Fatalf("stale attempt changed current session: %+v", status)
	}
}
