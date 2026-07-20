package subagentspawn

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/kernel"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(100, 0).UTC() }

func TestServiceAdmitsAuthenticatedReplayExactlyOnce(t *testing.T) {
	manager := kernel.NewLocalSessionManager(fixedClock{})
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
	manager := kernel.NewLocalSessionManager(fixedClock{})
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
