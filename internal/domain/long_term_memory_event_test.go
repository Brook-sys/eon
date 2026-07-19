package domain

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryStoredEventBuildsCanonicalEvent(t *testing.T) {
	at := time.Date(2026, 7, 19, 20, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	event, err := (MemoryStoredEvent{
		MemoryID: "memory-1",
		Key:      "mission context",
		Scope:    MemoryScopeMission,
		At:       at,
	}).Event("event-1")
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != EventMemoryStored || event.OccurredAt != at.UTC() || event.Sequence != 0 {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.PayloadRef != "memory_id=memory-1;key=mission+context;scope=mission" {
		t.Fatalf("unexpected payload ref: %q", event.PayloadRef)
	}
	if err := event.ValidateForAppend(); err != nil {
		t.Fatalf("event must satisfy append contract: %v", err)
	}
}

func TestMemoryCompactedEventBuildsCanonicalEvent(t *testing.T) {
	at := time.Date(2026, 7, 19, 23, 30, 0, 0, time.UTC)
	event, err := (MemoryCompactedEvent{
		MemoryID: "memory-1",
		Reason:   "operator delete",
		At:       at,
	}).Event("event-2")
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != EventMemoryCompacted || event.OccurredAt != at {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.PayloadRef != "memory_id=memory-1;reason=operator+delete" {
		t.Fatalf("unexpected payload ref: %q", event.PayloadRef)
	}
}

func TestMemoryEventsRejectIncompleteOrInvalidPayload(t *testing.T) {
	at := time.Date(2026, 7, 19, 23, 45, 0, 0, time.UTC)
	cases := []struct {
		name string
		fn   func() error
	}{
		{"stored missing event id", func() error {
			_, err := (MemoryStoredEvent{MemoryID: "m", Key: "k", Scope: MemoryScopeAgent, At: at}).Event("")
			return err
		}},
		{"stored invalid scope", func() error {
			_, err := (MemoryStoredEvent{MemoryID: "m", Key: "k", Scope: "other", At: at}).Event("e")
			return err
		}},
		{"compacted missing reason", func() error { _, err := (MemoryCompactedEvent{MemoryID: "m", At: at}).Event("e"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil || !strings.Contains(err.Error(), "memory") {
				t.Fatalf("expected memory event validation error, got %v", err)
			}
		})
	}
}
