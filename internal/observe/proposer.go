// Package observe persists bounded observation proposals anchored to immutable
// source fragments. Proposals are not canonical knowledge until a validated
// ProposedChangeSet commits them.
package observe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
)

const DefaultMaxStatementBytes = 4096

type Candidate struct {
	SourceFragmentID domain.SourceFragmentID
	Statement        string
	ExactQuote       string
	Provenance       string
}

type Result struct {
	Observation domain.Observation
	Event       domain.Event
}

type Proposer struct {
	Store             port.Store
	Clock             runtimesource.Clock
	IDs               runtimesource.IDGenerator
	MaxStatementBytes int
}

func (p Proposer) Propose(ctx context.Context, missionRevision domain.MissionRevisionID, candidate Candidate) (Result, error) {
	if p.Store == nil || p.Clock == nil || p.IDs == nil {
		return Result{}, errors.New("observation proposer requires store, clock and ID generator")
	}
	limit := p.MaxStatementBytes
	if limit == 0 {
		limit = DefaultMaxStatementBytes
	}
	if limit < 1 || strings.TrimSpace(candidate.Statement) == "" || len(candidate.Statement) > limit {
		return Result{}, fmt.Errorf("observation statement size %d is outside limit 1..%d", len(candidate.Statement), limit)
	}
	if candidate.SourceFragmentID == "" || candidate.ExactQuote == "" || strings.TrimSpace(candidate.Provenance) == "" {
		return Result{}, errors.New("fragment, exact quote and provenance are required")
	}

	observationID, err := p.IDs.NewID("observation")
	if err != nil {
		return Result{}, fmt.Errorf("generate observation ID: %w", err)
	}
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return Result{}, fmt.Errorf("generate event ID: %w", err)
	}
	result := Result{
		Observation: domain.Observation{SchemaVersion: 1, ID: domain.ObservationID(observationID), Statement: candidate.Statement, ExactQuote: candidate.ExactQuote, Anchor: domain.ObservationAnchor{SourceFragmentID: candidate.SourceFragmentID}, Provenance: candidate.Provenance},
		Event:       domain.Event{SchemaVersion: 1, ID: domain.EventID(eventID), Kind: "observation.proposed", OccurredAt: p.Clock.Now(), MissionRevision: missionRevision, PayloadRef: observationID},
	}
	err = p.Store.Update(ctx, func(tx port.Transaction) error {
		if missionRevision != "" {
			if _, err := tx.MissionRevision(missionRevision); err != nil {
				return err
			}
		}
		if err := tx.AppendObservation(result.Observation); err != nil {
			return err
		}
		persisted, err := tx.AppendEvent(result.Event)
		if err != nil {
			return err
		}
		result.Event = persisted
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("persist observation proposal: %w", err)
	}
	return result, nil
}
