// Package control implements narrow transport-facing inboxes. Records accepted
// here remain untrusted stimuli until a kernel processor applies domain rules.
package control

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// ExternalEventInbox persists untrusted stimuli through the store. Transports
// may only create RECEIVED dispositions; kernel processors own terminal states.
type ExternalEventInbox struct {
	Store       port.Store
	Disposition DispositionFactory
}

// DispositionFactory builds the initial RECEIVED disposition for an external
// event. Kernel processors later advance that disposition.
type DispositionFactory func(event domain.ExternalEvent) (domain.ExternalEventDisposition, error)

func NewExternalEventInbox(store port.Store, dispositions DispositionFactory) (*ExternalEventInbox, error) {
	if store == nil || dispositions == nil {
		return nil, fmt.Errorf("external event inbox requires store and disposition factory")
	}
	return &ExternalEventInbox{Store: store, Disposition: dispositions}, nil
}

func (i *ExternalEventInbox) SubmitExternalEvent(event domain.ExternalEvent) (domain.ExternalEventDisposition, error) {
	if err := event.Validate(); err != nil {
		return domain.ExternalEventDisposition{}, fmt.Errorf("validate external event: %w", err)
	}
	disposition, err := i.Disposition(event)
	if err != nil {
		return domain.ExternalEventDisposition{}, fmt.Errorf("build external event disposition: %w", err)
	}
	if err := disposition.Validate(); err != nil {
		return domain.ExternalEventDisposition{}, fmt.Errorf("validate external event disposition: %w", err)
	}
	if disposition.EventID != event.ID || disposition.State != domain.ExternalEventReceived {
		return domain.ExternalEventDisposition{}, fmt.Errorf("disposition factory must emit RECEIVED state for the event")
	}
	var stored domain.ExternalEventDisposition
	err = i.Store.Update(context.Background(), func(tx port.Transaction) error {
		if existing, lookupErr := tx.ExternalEvent(event.ID); lookupErr == nil {
			if !equalExternalEvents(existing, event) {
				return fmt.Errorf("%w: external event ID reused with different content", port.ErrConflict)
			}
			current, err := tx.ExternalEventDisposition(event.ID)
			if err != nil {
				return err
			}
			stored = current
			return nil
		} else if !errors.Is(lookupErr, port.ErrNotFound) {
			return lookupErr
		}
		if existing, lookupErr := tx.ExternalEventByDeduplicationKey(event.DeduplicationKey); lookupErr == nil {
			if !equalExternalEvents(existing, event) {
				return fmt.Errorf("%w: external event deduplication key reused with different content", port.ErrConflict)
			}
			current, err := tx.ExternalEventDisposition(existing.ID)
			if err != nil {
				return err
			}
			stored = current
			return nil
		} else if !errors.Is(lookupErr, port.ErrNotFound) {
			return lookupErr
		}
		if err := tx.CreateExternalEvent(event, disposition); err != nil {
			return err
		}
		stored = disposition
		return nil
	})
	if err != nil {
		return domain.ExternalEventDisposition{}, err
	}
	return stored, nil
}

func (i *ExternalEventInbox) ExternalEvent(id domain.ExternalEventID) (domain.ExternalEvent, error) {
	var event domain.ExternalEvent
	err := i.Store.View(context.Background(), func(r port.Reader) error {
		got, err := r.ExternalEvent(id)
		if err != nil {
			return err
		}
		event = got
		return nil
	})
	return event, err
}

func (i *ExternalEventInbox) ExternalEventByDeduplicationKey(key string) (domain.ExternalEvent, error) {
	if key == "" {
		return domain.ExternalEvent{}, errors.New("external event deduplication key is required")
	}
	var event domain.ExternalEvent
	err := i.Store.View(context.Background(), func(r port.Reader) error {
		got, err := r.ExternalEventByDeduplicationKey(key)
		if err != nil {
			return err
		}
		event = got
		return nil
	})
	return event, err
}

func (i *ExternalEventInbox) ExternalEventDisposition(id domain.ExternalEventID) (domain.ExternalEventDisposition, error) {
	var disposition domain.ExternalEventDisposition
	err := i.Store.View(context.Background(), func(r port.Reader) error {
		got, err := r.ExternalEventDisposition(id)
		if err != nil {
			return err
		}
		disposition = got
		return nil
	})
	return disposition, err
}

// FixedDispositionFactory is useful in tests that need deterministic timestamps.
func FixedDispositionFactory(recordedAt time.Time) DispositionFactory {
	return func(event domain.ExternalEvent) (domain.ExternalEventDisposition, error) {
		return domain.ExternalEventDisposition{
			SchemaVersion: domain.SchemaVersionV1,
			EventID:       event.ID,
			State:         domain.ExternalEventReceived,
			RecordedAt:    recordedAt.UTC(),
		}, nil
	}
}

func equalExternalEvents(a, b domain.ExternalEvent) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ID != b.ID || a.DeduplicationKey != b.DeduplicationKey || a.Source != b.Source || a.SourceActorID != b.SourceActorID || a.Kind != b.Kind || a.MissionID != b.MissionID || a.CorrelationID != b.CorrelationID || a.TransportMessageID != b.TransportMessageID || !a.ReceivedAt.Equal(b.ReceivedAt) {
		return false
	}
	if a.Content.MediaType != b.Content.MediaType || a.Content.Text != b.Content.Text || a.Content.Reference != b.Content.Reference {
		return false
	}
	return string(a.Content.Structured) == string(b.Content.Structured)
}

var _ port.ExternalEventInbox = (*ExternalEventInbox)(nil)
