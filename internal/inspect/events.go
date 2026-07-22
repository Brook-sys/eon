package inspect

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	DefaultEventPageLimit = 50
	MaxEventPageLimit     = 200
)

// EventFilter selects a bounded page of the append-only event log.
type EventFilter struct {
	AfterSequence   uint64
	Limit           int
	Kind            string
	Namespace       string
	MissionRevision domain.MissionRevisionID
	InquiryID       domain.InquiryID
	OperationID     domain.OperationID
	CommitID        domain.CommitID
}

// EventPage is a resumable timeline slice.
type EventPage struct {
	SchemaVersion int            `json:"schema_version"`
	AfterSequence uint64         `json:"after_sequence"`
	Limit         int            `json:"limit"`
	NextSequence  uint64         `json:"next_sequence"`
	HasMore       bool           `json:"has_more"`
	Events        []domain.Event `json:"events"`
	FilterApplied bool           `json:"filter_applied"`
	GeneratedAt   time.Time      `json:"generated_at"`
}

// ListEvents returns events after a sequence with optional correlation filters.
// Filtering is applied in the projection layer; the store remains a pure log.
func (p *Projector) ListEvents(ctx context.Context, filter EventFilter) (EventPage, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultEventPageLimit
	}
	if limit > MaxEventPageLimit {
		return EventPage{}, fmt.Errorf("event page limit must be <= %d", MaxEventPageLimit)
	}
	filterApplied := filter.Kind != "" || filter.Namespace != "" || filter.MissionRevision != "" || filter.InquiryID != "" || filter.OperationID != "" || filter.CommitID != ""

	var page EventPage
	err := p.Store.View(ctx, func(r port.Reader) error {
		// A filtered page may be sparse, so scan bounded batches until the page
		// is full and one later match is proven, the log ends, or the global
		// scan ceiling is reached. The continuation cursor advances across
		// examined non-matches so sparse filters always make progress.
		after := filter.AfterSequence
		matched := make([]domain.Event, 0, limit)
		lastSeen := after
		scanned := 0
		hasMore := false
		stoppedBeforeMatch := false
		for scanned < maxCorrelationEventScan {
			batchLimit := MaxEventPageLimit
			if remaining := maxCorrelationEventScan - scanned; remaining < batchLimit {
				batchLimit = remaining
			}
			batch, err := r.Events(after, batchLimit)
			if err != nil {
				return err
			}
			if len(batch) == 0 {
				break
			}
			for _, event := range batch {
				lastSeen = event.Sequence
				if !eventMatches(event, filter) {
					continue
				}
				if len(matched) == limit {
					// This match belongs to the next page. Resume from the last
					// returned match so it cannot be skipped.
					hasMore = true
					stoppedBeforeMatch = true
					break
				}
				matched = append(matched, event)
			}
			if stoppedBeforeMatch {
				break
			}
			scanned += len(batch)
			after = batch[len(batch)-1].Sequence
			if len(batch) < batchLimit {
				break
			}
		}

		next := lastSeen
		if stoppedBeforeMatch {
			next = matched[len(matched)-1].Sequence
		} else if scanned >= maxCorrelationEventScan {
			rest, err := r.Events(after, 1)
			if err != nil {
				return err
			}
			hasMore = len(rest) > 0
		}
		page = EventPage{
			SchemaVersion: domain.SchemaVersionV1,
			AfterSequence: filter.AfterSequence,
			Limit:         limit,
			NextSequence:  next,
			HasMore:       hasMore,
			Events:        matched,
			FilterApplied: filterApplied,
			GeneratedAt:   p.Clock().UTC(),
		}
		return nil
	})
	if err != nil {
		return EventPage{}, err
	}
	return page, nil
}

func eventMatches(event domain.Event, filter EventFilter) bool {
	if filter.Kind != "" && event.Kind != filter.Kind {
		return false
	}
	if filter.Namespace != "" && event.Namespace != filter.Namespace {
		return false
	}
	if filter.MissionRevision != "" && event.MissionRevision != filter.MissionRevision {
		return false
	}
	if filter.InquiryID != "" && event.InquiryID != filter.InquiryID {
		return false
	}
	if filter.OperationID != "" && event.OperationID != filter.OperationID {
		return false
	}
	if filter.CommitID != "" && event.CommitID != filter.CommitID {
		return false
	}
	return true
}

// GetEvent loads one immutable event by ID.
func (p *Projector) GetEvent(ctx context.Context, id domain.EventID) (domain.Event, error) {
	if id == "" {
		return domain.Event{}, errors.New("event ID is required")
	}
	var event domain.Event
	err := p.Store.View(ctx, func(r port.Reader) error {
		got, err := r.EventByID(id)
		if err != nil {
			return err
		}
		event = got
		return nil
	})
	return event, err
}
