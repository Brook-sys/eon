package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

const (
	EventQuestionGateAdmitted   = "operator.question.gate.admitted"
	EventQuestionGateSuppressed = "operator.question.gate.suppressed"
	EventQuestionGateDeferred   = "operator.question.gate.deferred"
)

type QuestionRoute struct {
	Channel        string
	DestinationRef string
	MaxAttempts    uint32
}

func (r QuestionRoute) Validate() error {
	if r.Channel == "" || r.DestinationRef == "" || r.MaxAttempts == 0 {
		return errors.New("question route requires channel, destination, and max attempts")
	}
	return nil
}

// QuestionGateProcessor evaluates a proposal against durable observations and
// atomically records the decision. ADMIT additionally creates the canonical
// question and one delivery per configured route in the same transaction.
type QuestionGateProcessor struct {
	Store         port.Store
	Clock         source.Clock
	IDs           source.IDGenerator
	Policy        QuestionGatePolicy
	PolicyVersion string
	Routes        []QuestionRoute
}

func NewQuestionGateProcessor(store port.Store, clock source.Clock, ids source.IDGenerator, policy QuestionGatePolicy, policyVersion string, routes []QuestionRoute) (*QuestionGateProcessor, error) {
	if store == nil || clock == nil || ids == nil || policyVersion == "" {
		return nil, errors.New("question gate processor requires store, clock, IDs, and policy version")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	for i, route := range routes {
		if err := route.Validate(); err != nil {
			return nil, fmt.Errorf("validate question route %d: %w", i, err)
		}
	}
	return &QuestionGateProcessor{Store: store, Clock: clock, IDs: ids, Policy: policy, PolicyVersion: policyVersion, Routes: append([]QuestionRoute(nil), routes...)}, nil
}

func (p *QuestionGateProcessor) Process(ctx context.Context, proposal domain.OperatorQuestionProposal) (domain.QuestionGateDecisionRecord, error) {
	if err := proposal.Validate(); err != nil {
		return domain.QuestionGateDecisionRecord{}, err
	}
	var existing domain.QuestionGateDecisionRecord
	if err := p.Store.View(ctx, func(r port.Reader) error {
		decision, err := r.QuestionGateDecisionByQuestion(proposal.Question.ID)
		if errors.Is(err, port.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		existing = decision
		return nil
	}); err != nil {
		return domain.QuestionGateDecisionRecord{}, err
	}
	if existing.ID != "" {
		if existing.MissionID != proposal.Question.MissionID || existing.DedupSignature != proposal.Question.DedupSignature {
			return domain.QuestionGateDecisionRecord{}, fmt.Errorf("%w: proposal identity differs from recorded gate decision", port.ErrConflict)
		}
		return existing, nil
	}

	now := p.Clock.Now().UTC()
	decisionID, err := p.IDs.NewID("question_gate")
	if err != nil {
		return domain.QuestionGateDecisionRecord{}, err
	}
	var final domain.QuestionGateDecisionRecord
	err = p.Store.Update(ctx, func(tx port.Transaction) error {
		if replay, err := tx.QuestionGateDecisionByQuestion(proposal.Question.ID); err == nil {
			final = replay
			return nil
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		history, err := questionGateHistory(tx, proposal.Question.MissionID)
		if err != nil {
			return err
		}
		result, err := EvaluateQuestion(p.Policy, now, proposal, history)
		if err != nil {
			return err
		}
		final = persistedQuestionGateDecision(decisionID, proposal, result, p.PolicyVersion, now)
		if err := tx.CreateQuestionGateDecision(final); err != nil {
			return err
		}
		if result.Decision == QuestionAdmit {
			if err := tx.CreateOperatorQuestion(proposal.Question); err != nil {
				return err
			}
			availableAt := result.DeliveryAvailable
			if availableAt.IsZero() || availableAt.Before(now) {
				availableAt = now
			}
			for _, route := range p.Routes {
				deliveryID, err := p.IDs.NewID("question_delivery")
				if err != nil {
					return err
				}
				delivery := domain.QuestionDelivery{
					SchemaVersion: domain.SchemaVersionV1, ID: domain.QuestionDeliveryID(deliveryID),
					QuestionID: proposal.Question.ID, QuestionRevision: proposal.Question.Revision,
					Channel: route.Channel, DestinationRef: route.DestinationRef,
					Status: domain.QuestionDeliveryPending, MaxAttempts: route.MaxAttempts,
					AvailableAt: availableAt, CreatedAt: now, UpdatedAt: now,
				}
				if err := tx.CreateQuestionDelivery(delivery); err != nil {
					return err
				}
			}
		}
		eventID, err := p.IDs.NewID("event")
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(eventID),
			Kind: questionGateEventKind(result.Decision), OccurredAt: now,
			MissionRevision: proposal.Question.MissionRevision,
			PayloadRef:      string(final.ID) + ":" + string(final.QuestionID) + ":" + string(final.Reason),
		})
		return err
	})
	if err != nil {
		return domain.QuestionGateDecisionRecord{}, err
	}
	return final, nil
}

func questionGateHistory(r port.Reader, missionID domain.MissionID) ([]QuestionGateRecord, error) {
	questions, err := r.OperatorQuestions(missionID, "")
	if err != nil {
		return nil, err
	}
	history := make([]QuestionGateRecord, 0, len(questions))
	for _, question := range questions {
		deliveries, err := r.QuestionDeliveries(question.ID)
		if err != nil {
			return nil, err
		}
		var deliveredAt time.Time
		for _, delivery := range deliveries {
			if delivery.Status == domain.QuestionDeliveryDelivered && (deliveredAt.IsZero() || delivery.UpdatedAt.Before(deliveredAt)) {
				deliveredAt = delivery.UpdatedAt
			}
		}
		var closedAt time.Time
		if question.Status.Terminal() {
			closedAt = question.AnsweredAt
			if closedAt.IsZero() {
				closedAt = question.ExpiresAt
			}
			if closedAt.IsZero() {
				closedAt = question.CreatedAt
			}
		}
		history = append(history, QuestionGateRecord{
			QuestionID: question.ID, MissionID: question.MissionID, DedupSignature: question.DedupSignature,
			Status: question.Status, DeliveredAt: deliveredAt, ClosedAt: closedAt, AdmittedAt: question.CreatedAt,
		})
	}
	return history, nil
}

func persistedQuestionGateDecision(id string, proposal domain.OperatorQuestionProposal, result QuestionGateResult, policyVersion string, now time.Time) domain.QuestionGateDecisionRecord {
	return domain.QuestionGateDecisionRecord{
		SchemaVersion: domain.SchemaVersionV1, ID: domain.QuestionGateDecisionID(id),
		QuestionID: proposal.Question.ID, MissionID: proposal.Question.MissionID, DedupSignature: proposal.Question.DedupSignature,
		Decision: domain.PersistedQuestionGateDecision(result.Decision), Reason: domain.PersistedQuestionGateReason(result.Reason),
		PolicyVersion: policyVersion, EvaluatedAt: now, RetryAfter: result.RetryAfter,
	}
}

func questionGateEventKind(decision QuestionGateDecision) string {
	switch decision {
	case QuestionAdmit:
		return EventQuestionGateAdmitted
	case QuestionSuppress:
		return EventQuestionGateSuppressed
	default:
		return EventQuestionGateDeferred
	}
}
