package domain

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// MemoryStoredEvent is the typed payload projected into the canonical Event log
// when semantic memory is created or replaced.
type MemoryStoredEvent struct {
	MemoryID MemoryID
	Key      string
	Scope    MemoryScope
	At       time.Time
}

func (e MemoryStoredEvent) Event(id EventID) (Event, error) {
	if id == "" || e.MemoryID == "" || e.Key == "" || e.At.IsZero() {
		return Event{}, errors.New("memory stored event is incomplete")
	}
	if !validMemoryScope(e.Scope) {
		return Event{}, errors.New("memory stored event has invalid scope")
	}
	return Event{
		SchemaVersion: SchemaVersionV1,
		ID:            id,
		Kind:          EventMemoryStored,
		OccurredAt:    e.At.UTC(),
		PayloadRef: fmt.Sprintf("memory_id=%s;key=%s;scope=%s",
			url.QueryEscape(string(e.MemoryID)), url.QueryEscape(e.Key), url.QueryEscape(string(e.Scope))),
	}, nil
}

// MemoryCompactedEvent records removal from the current semantic-memory view.
// The append-only event remains the durable audit fact.
type MemoryCompactedEvent struct {
	MemoryID MemoryID
	Reason   string
	At       time.Time
}

func (e MemoryCompactedEvent) Event(id EventID) (Event, error) {
	if id == "" || e.MemoryID == "" || e.Reason == "" || e.At.IsZero() {
		return Event{}, errors.New("memory compacted event is incomplete")
	}
	return Event{
		SchemaVersion: SchemaVersionV1,
		ID:            id,
		Kind:          EventMemoryCompacted,
		OccurredAt:    e.At.UTC(),
		PayloadRef: fmt.Sprintf("memory_id=%s;reason=%s",
			url.QueryEscape(string(e.MemoryID)), url.QueryEscape(e.Reason)),
	}, nil
}

func validMemoryScope(scope MemoryScope) bool {
	switch scope {
	case MemoryScopeMission, MemoryScopeStrategy, MemoryScopeAgent:
		return true
	default:
		return false
	}
}
