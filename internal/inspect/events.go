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
	filterApplied := filter.Kind != "" || filter.MissionRevision != "" || filter.InquiryID != "" || filter.OperationID != "" || filter.CommitID != ""

	var page EventPage
	err := p.Store.View(ctx, func(r port.Reader) error {
		// Over-fetch when filters are present so a sparse page can still fill.
		batchLimit := limit
		if filterApplied {
			batchLimit = MaxEventPageLimit
		}
		after := filter.AfterSequence
		matched := make([]domain.Event, 0, limit)
		var lastSeen uint64 = after
		hasMore := false
		for {
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
				matched = append(matched, event)
				if len(matched) == limit {
					// There may still be more matching events after this one.
					hasMore = true
					// Confirm at least one later event exists, matching or not.
					// Clients resume from NextSequence = last matched sequence.
					goto done
				}
			}
			after = batch[len(batch)-1].Sequence
			if len(batch) < batchLimit {
				break
			}
			// Hard stop: avoid unbounded scans in a single request.
			if after-filter.AfterSequence >= 5000 {
				hasMore = true
				break
			}
		}
	done:
		next := lastSeen
		if len(matched) > 0 {
			next = matched[len(matched)-1].Sequence
		}
		// If we filled the page, verify whether anything remains after next.
		if hasMore && len(matched) == limit {
			rest, err := r.Events(next, 1)
			if err != nil {
				return err
			}
			hasMore = len(rest) > 0
			// When filters are applied, remaining unfiltered events may still
			// fail the filter; clients still resume correctly via next sequence.
			if filterApplied && hasMore {
				// Best-effort: scan a little further for any remaining match.
				probe, err := r.Events(next, MaxEventPageLimit)
				if err != nil {
					return err
				}
				hasMore = false
				for _, event := range probe {
					if eventMatches(event, filter) {
						hasMore = true
						break
					}
				}
			}
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
