// Package control implements narrow transport-facing inboxes. Records accepted
// here remain untrusted stimuli until a kernel processor applies domain rules.
package control

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type ExternalEventInbox struct {
	mu      sync.RWMutex
	byID    map[domain.ExternalEventID]domain.ExternalEvent
	byDedup map[string]domain.ExternalEventID
}

func NewExternalEventInbox() *ExternalEventInbox {
	return &ExternalEventInbox{byID: make(map[domain.ExternalEventID]domain.ExternalEvent), byDedup: make(map[string]domain.ExternalEventID)}
}

func (i *ExternalEventInbox) SubmitExternalEvent(event domain.ExternalEvent) (domain.ExternalEvent, error) {
	if err := event.Validate(); err != nil {
		return domain.ExternalEvent{}, fmt.Errorf("validate external event: %w", err)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if existing, ok := i.byID[event.ID]; ok {
		if equalExternalEvents(existing, event) {
			return cloneExternalEvent(existing), nil
		}
		return domain.ExternalEvent{}, fmt.Errorf("%w: external event ID reused with different content", port.ErrConflict)
	}
	if existingID, ok := i.byDedup[event.DeduplicationKey]; ok {
		existing := i.byID[existingID]
		if equalExternalEvents(existing, event) {
			return cloneExternalEvent(existing), nil
		}
		return domain.ExternalEvent{}, fmt.Errorf("%w: external event deduplication key reused with different content", port.ErrConflict)
	}
	stored := cloneExternalEvent(event)
	i.byID[event.ID] = stored
	i.byDedup[event.DeduplicationKey] = event.ID
	return cloneExternalEvent(stored), nil
}

func (i *ExternalEventInbox) ExternalEvent(id domain.ExternalEventID) (domain.ExternalEvent, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	event, ok := i.byID[id]
	if !ok {
		return domain.ExternalEvent{}, fmt.Errorf("%w: external event %s", port.ErrNotFound, id)
	}
	return cloneExternalEvent(event), nil
}

func (i *ExternalEventInbox) ExternalEventByDeduplicationKey(key string) (domain.ExternalEvent, error) {
	if key == "" {
		return domain.ExternalEvent{}, errors.New("external event deduplication key is required")
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	id, ok := i.byDedup[key]
	if !ok {
		return domain.ExternalEvent{}, fmt.Errorf("%w: external event deduplication key %s", port.ErrNotFound, key)
	}
	return cloneExternalEvent(i.byID[id]), nil
}

func equalExternalEvents(a, b domain.ExternalEvent) bool { return reflect.DeepEqual(a, b) }

func cloneExternalEvent(event domain.ExternalEvent) domain.ExternalEvent {
	event.Content.Structured = append([]byte(nil), event.Content.Structured...)
	return event
}

var _ port.ExternalEventInbox = (*ExternalEventInbox)(nil)
