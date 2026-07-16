package control

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func validExternalEvent() domain.ExternalEvent {
	return domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "external_1", DeduplicationKey: "telegram:update:42", Source: "telegram",
		SourceActorID: "operator_1", Kind: domain.ExternalUserAnswer, MissionID: "mission_1", CorrelationID: "ask_1",
		TransportMessageID: "message_7", Content: domain.ExternalContent{MediaType: "application/json", Structured: json.RawMessage(`{"answer_id":"answer_1"}`)},
		ReceivedAt: time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
	}
}

func TestExternalEventInboxReplaysIdenticalDelivery(t *testing.T) {
	inbox := NewExternalEventInbox()
	event := validExternalEvent()
	first, err := inbox.SubmitExternalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := inbox.SubmitExternalEvent(event)
	if err != nil || first.ID != second.ID {
		t.Fatalf("replay = %#v, err = %v", second, err)
	}
	byKey, err := inbox.ExternalEventByDeduplicationKey(event.DeduplicationKey)
	if err != nil || byKey.ID != event.ID {
		t.Fatalf("by key = %#v, err = %v", byKey, err)
	}
	byKey.Content.Structured[0] = 'X'
	stored, err := inbox.ExternalEvent(event.ID)
	if err != nil || string(stored.Content.Structured) != string(event.Content.Structured) {
		t.Fatal("caller mutated stored external event")
	}
}

func TestExternalEventInboxRejectsDivergentReuse(t *testing.T) {
	inbox := NewExternalEventInbox()
	event := validExternalEvent()
	if _, err := inbox.SubmitExternalEvent(event); err != nil {
		t.Fatal(err)
	}
	changedID := event
	changedID.SourceActorID = "attacker"
	if _, err := inbox.SubmitExternalEvent(changedID); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("divergent ID error = %v", err)
	}
	changedKey := event
	changedKey.ID = "external_2"
	if _, err := inbox.SubmitExternalEvent(changedKey); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("divergent key error = %v", err)
	}
}

func TestExternalEventInboxRejectsInvalidAndMissingRecords(t *testing.T) {
	inbox := NewExternalEventInbox()
	if _, err := inbox.SubmitExternalEvent(domain.ExternalEvent{}); err == nil {
		t.Fatal("invalid event accepted")
	}
	if _, err := inbox.ExternalEvent("missing"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("missing ID error = %v", err)
	}
	if _, err := inbox.ExternalEventByDeduplicationKey("missing"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("missing key error = %v", err)
	}
}
