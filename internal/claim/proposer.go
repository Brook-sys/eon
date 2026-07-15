// Package claim persists bounded claim proposals and their typed evidence
// links. These records remain proposals until promoted through a validated
// ProposedChangeSet.
package claim

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	runtimesource "motor-autonomo/internal/runtime/source"
)

const DefaultMaxPropositionBytes = 4096

type EvidenceCandidate struct {
	ObservationID domain.ObservationID
	Relation      domain.EvidenceRelation
	Rationale     string
}

type Candidate struct {
	Proposition string
	Qualifiers  map[string]string
	Evidence    []EvidenceCandidate
}

type Result struct {
	Claim         domain.Claim
	EvidenceLinks []domain.EvidenceLink
	Event         domain.Event
}

type Proposer struct {
	Store               port.Store
	Clock               runtimesource.Clock
	IDs                 runtimesource.IDGenerator
	MaxPropositionBytes int
}

func (p Proposer) Propose(ctx context.Context, missionRevision domain.MissionRevisionID, candidate Candidate) (Result, error) {
	if p.Store == nil || p.Clock == nil || p.IDs == nil {
		return Result{}, errors.New("claim proposer requires store, clock and ID generator")
	}
	limit := p.MaxPropositionBytes
	if limit == 0 {
		limit = DefaultMaxPropositionBytes
	}
	if limit < 1 || strings.TrimSpace(candidate.Proposition) == "" || len(candidate.Proposition) > limit {
		return Result{}, fmt.Errorf("claim proposition size %d is outside limit 1..%d", len(candidate.Proposition), limit)
	}
	if len(candidate.Evidence) == 0 {
		return Result{}, errors.New("claim proposal requires evidence")
	}
	claimID, err := p.IDs.NewID("claim")
	if err != nil {
		return Result{}, fmt.Errorf("generate claim ID: %w", err)
	}
	result := Result{Claim: domain.Claim{SchemaVersion: 1, ID: domain.ClaimID(claimID), Proposition: candidate.Proposition, Qualifiers: cloneQualifiers(candidate.Qualifiers), Version: 1}}
	for _, evidence := range candidate.Evidence {
		linkID, err := p.IDs.NewID("evidence_link")
		if err != nil {
			return Result{}, fmt.Errorf("generate evidence link ID: %w", err)
		}
		result.EvidenceLinks = append(result.EvidenceLinks, domain.EvidenceLink{SchemaVersion: 1, ID: domain.EvidenceLinkID(linkID), ObservationID: evidence.ObservationID, ClaimID: result.Claim.ID, Relation: evidence.Relation, Rationale: evidence.Rationale})
	}
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return Result{}, fmt.Errorf("generate event ID: %w", err)
	}
	result.Event = domain.Event{SchemaVersion: 1, ID: domain.EventID(eventID), Kind: "claim.proposed", OccurredAt: p.Clock.Now(), MissionRevision: missionRevision, PayloadRef: claimID}
	err = p.Store.Update(ctx, func(tx port.Transaction) error {
		if missionRevision != "" {
			if _, err := tx.MissionRevision(missionRevision); err != nil {
				return err
			}
		}
		if err := tx.AppendClaimWithEvidence(result.Claim, result.EvidenceLinks); err != nil {
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
		return Result{}, fmt.Errorf("persist claim proposal: %w", err)
	}
	return result, nil
}

func cloneQualifiers(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
