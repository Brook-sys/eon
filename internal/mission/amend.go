// Package mission owns MissionSpec load and explicit UserAmendment acceptance.
package mission

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	EventMissionAmendmentAccepted    = "mission.amendment_accepted"
	EventMissionAgendaReconciled     = "mission.agenda_reconciled"
	EventMissionOperationCancelled   = "mission.operation_cancelled"
	EventMissionInquiryCancelled     = "mission.inquiry_cancelled"
	EventMissionOpportunityAbandoned = "mission.opportunity_abandoned"
)

// AmendmentAcceptance is the pure plan plus the durable results of accepting a
// UserAmendment (FR-AUTH-004). Previous MissionRevision rows remain intact.
type AmendmentAcceptance struct {
	Previous domain.MissionRevision
	Accepted domain.MissionRevision
	Diff     domain.MissionDiff
	Impact   domain.MissionImpactPreview
	Report   domain.AgendaReconciliationReport
}

// Acceptor installs a candidate MissionRevision after pure diff/impact and
// reconciles the agenda of the superseded revision in one storage transaction.
type Acceptor struct {
	Store port.Store
	Clock Clock
	IDs   IDGenerator
}

// Accept validates the amendment against the active revision, computes semantic
// diff and impact, appends+activates the new revision, cancels non-terminal
// units of the previous revision, abandons open/deferred work opportunities,
// and emits audit events. No-op and blocked impacts fail closed without writes.
func (a Acceptor) Accept(ctx context.Context, amendment domain.UserAmendment, provenance string) (AmendmentAcceptance, error) {
	if a.Store == nil || a.Clock == nil || a.IDs == nil {
		return AmendmentAcceptance{}, errors.New("mission acceptor dependencies are incomplete")
	}
	if strings.TrimSpace(provenance) == "" {
		return AmendmentAcceptance{}, errors.New("mission amendment provenance must not be empty")
	}
	if err := amendment.Validate(); err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("validate user amendment: %w", err)
	}

	var previous domain.MissionRevision
	if err := a.Store.View(ctx, func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(amendment.MissionID)
		if err != nil {
			return err
		}
		previous = active
		return nil
	}); err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("load active mission revision: %w", err)
	}

	candidate, err := domain.CandidateFromAmendment(previous, amendment)
	if err != nil {
		return AmendmentAcceptance{}, err
	}
	diff, err := domain.DiffMissionRevisions(previous, candidate)
	if err != nil {
		return AmendmentAcceptance{}, err
	}
	impact, err := domain.PreviewMissionImpact(previous, candidate, diff)
	if err != nil {
		return AmendmentAcceptance{}, err
	}
	if impact.Blocked || !impact.RequiresAcceptance {
		return AmendmentAcceptance{}, errors.New("mission amendment is blocked or does not require acceptance")
	}

	revisionID, err := a.IDs.NewID("mission_revision")
	if err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("generate mission revision ID: %w", err)
	}
	acceptEventID, err := a.IDs.NewID("event")
	if err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("generate acceptance event ID: %w", err)
	}
	reconcileEventID, err := a.IDs.NewID("event")
	if err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("generate reconcile event ID: %w", err)
	}
	now := a.Clock.Now().UTC()

	accepted := domain.MissionRevision{
		SchemaVersion:        domain.SchemaVersionV1,
		ID:                   domain.MissionRevisionID(revisionID),
		MissionID:            amendment.MissionID,
		Revision:             amendment.CandidateRevision,
		OriginalText:         amendment.OriginalText,
		Purpose:              amendment.Purpose,
		Domains:              append([]string(nil), amendment.Domains...),
		Policies:             append([]string(nil), amendment.Policies...),
		Budget:               amendment.Budget,
		Status:               amendment.Status,
		StandingObjectives:   append([]string(nil), amendment.StandingObjectives...),
		RecurringObligations: append([]domain.RecurringObligation(nil), amendment.RecurringObligations...),
		Provenance:           provenance,
		AcceptedAt:           now,
	}
	if err := accepted.Validate(); err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("build accepted mission revision: %w", err)
	}

	report := domain.AgendaReconciliationReport{
		PreviousRevision: previous.ID,
		NewRevision:      accepted.ID,
	}

	err = a.Store.Update(ctx, func(tx port.Transaction) error {
		// Optimistic re-check inside the write transaction.
		active, err := tx.ActiveMissionRevision(amendment.MissionID)
		if err != nil {
			return err
		}
		if active.ID != previous.ID || active.Revision != previous.Revision {
			return fmt.Errorf("%w: active mission revision changed during acceptance", port.ErrConflict)
		}
		if err := tx.AppendMissionRevision(accepted); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(accepted.MissionID, accepted.ID); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(acceptEventID),
			Kind:            EventMissionAmendmentAccepted,
			OccurredAt:      now,
			MissionRevision: accepted.ID,
			PayloadRef:      string(previous.ID),
		}); err != nil {
			return err
		}

		ops, err := tx.Operations(previous.ID)
		if err != nil {
			return err
		}
		for _, op := range ops {
			if op.State.Terminal() {
				continue
			}
			snap, err := domain.Transition(
				domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation},
				domain.TransitionInput{Event: domain.EventCancel, Reference: "mission_amendment"},
			)
			if err != nil {
				return fmt.Errorf("cancel operation %s: %w", op.ID, err)
			}
			op.State = snap.State
			op.Reevaluation = snap.Reevaluation
			if err := tx.SaveOperation(op); err != nil {
				return err
			}
			report.CancelledOperations = append(report.CancelledOperations, op.ID)
			eventID, err := a.IDs.NewID("event")
			if err != nil {
				return err
			}
			if _, err := tx.AppendEvent(domain.Event{
				SchemaVersion:   domain.SchemaVersionV1,
				ID:              domain.EventID(eventID),
				Kind:            EventMissionOperationCancelled,
				OccurredAt:      now,
				MissionRevision: previous.ID,
				OperationID:     op.ID,
				PayloadRef:      string(accepted.ID),
			}); err != nil {
				return err
			}
		}

		// Inquiries are cancelled via their own SaveInquiry surface when present
		// on the previous revision. We discover them through their operations and
		// through a direct scan of opportunities' mission revision binding.
		seenInquiries := map[domain.InquiryID]struct{}{}
		for _, op := range ops {
			if _, ok := seenInquiries[op.InquiryID]; ok {
				continue
			}
			seenInquiries[op.InquiryID] = struct{}{}
			inquiry, err := tx.Inquiry(op.InquiryID)
			if err != nil {
				return err
			}
			if inquiry.MissionRevision != previous.ID || inquiry.State.Terminal() {
				continue
			}
			snap, err := domain.Transition(
				domain.OperationalSnapshot{State: inquiry.State, Reevaluation: inquiry.Reevaluation},
				domain.TransitionInput{Event: domain.EventCancel, Reference: "mission_amendment"},
			)
			if err != nil {
				return fmt.Errorf("cancel inquiry %s: %w", inquiry.ID, err)
			}
			inquiry.State = snap.State
			inquiry.Reevaluation = snap.Reevaluation
			if err := tx.SaveInquiry(inquiry); err != nil {
				return err
			}
			report.CancelledInquiries = append(report.CancelledInquiries, inquiry.ID)
			eventID, err := a.IDs.NewID("event")
			if err != nil {
				return err
			}
			if _, err := tx.AppendEvent(domain.Event{
				SchemaVersion:   domain.SchemaVersionV1,
				ID:              domain.EventID(eventID),
				Kind:            EventMissionInquiryCancelled,
				OccurredAt:      now,
				MissionRevision: previous.ID,
				InquiryID:       inquiry.ID,
				PayloadRef:      string(accepted.ID),
			}); err != nil {
				return err
			}
		}

		for _, status := range []domain.WorkOpportunityStatus{domain.OpportunityOpen, domain.OpportunityDeferred} {
			opps, err := tx.WorkOpportunities(previous.ID, status)
			if err != nil {
				return err
			}
			for _, opp := range opps {
				next, err := domain.TransitionWorkOpportunity(opp, domain.WorkOpportunityTransition{
					Event:      domain.OppEventAbandon,
					OccurredAt: now,
					Reason:     "mission_amendment:" + string(accepted.ID),
				})
				if err != nil {
					return fmt.Errorf("abandon work opportunity %s: %w", opp.ID, err)
				}
				if err := tx.SaveWorkOpportunity(next); err != nil {
					return err
				}
				report.AbandonedOpportunities = append(report.AbandonedOpportunities, opp.ID)
				eventID, err := a.IDs.NewID("event")
				if err != nil {
					return err
				}
				if _, err := tx.AppendEvent(domain.Event{
					SchemaVersion:   domain.SchemaVersionV1,
					ID:              domain.EventID(eventID),
					Kind:            EventMissionOpportunityAbandoned,
					OccurredAt:      now,
					MissionRevision: previous.ID,
					PayloadRef:      string(opp.ID),
				}); err != nil {
					return err
				}
			}
		}

		if err := report.Validate(); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(reconcileEventID),
			Kind:            EventMissionAgendaReconciled,
			OccurredAt:      now,
			MissionRevision: accepted.ID,
			PayloadRef:      string(previous.ID),
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return AmendmentAcceptance{}, fmt.Errorf("accept mission amendment: %w", err)
	}

	return AmendmentAcceptance{
		Previous: previous,
		Accepted: accepted,
		Diff:     diff,
		Impact:   impact,
		Report:   report,
	}, nil
}
