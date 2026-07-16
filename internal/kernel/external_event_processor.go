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
	EventExternalEventReceived    = "external.event.received"
	EventExternalEventApplied     = "external.event.applied"
	EventExternalEventRejected    = "external.event.rejected"
	EventExternalEventIgnored     = "external.event.ignored"
	EventOperatorQuestionAnswered = "operator.question.answered"
	EventExternalWakeObserved     = "external.wake.observed"
)

// ExternalEventProcessor applies durable untrusted stimuli. Content never
// becomes policy: only typed domain transitions and wake resumes are allowed.
type ExternalEventProcessor struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
}

func NewExternalEventProcessor(store port.Store, clock source.Clock, ids source.IDGenerator) (*ExternalEventProcessor, error) {
	if store == nil || clock == nil || ids == nil {
		return nil, errors.New("external event processor requires store, clock, and ID generator")
	}
	return &ExternalEventProcessor{Store: store, Clock: clock, IDs: ids}, nil
}

// ProcessNext applies at most one pending external event.
func (p *ExternalEventProcessor) ProcessNext(ctx context.Context) (domain.ExternalEventDisposition, bool, error) {
	var eventID domain.ExternalEventID
	err := p.Store.View(ctx, func(r port.Reader) error {
		pending, err := r.PendingExternalEvents(1)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		eventID = pending[0].ID
		return nil
	})
	if err != nil {
		return domain.ExternalEventDisposition{}, false, err
	}
	if eventID == "" {
		return domain.ExternalEventDisposition{}, false, nil
	}
	disposition, err := p.Process(ctx, eventID)
	return disposition, true, err
}

// Process applies one external event idempotently. Terminal dispositions replay.
func (p *ExternalEventProcessor) Process(ctx context.Context, eventID domain.ExternalEventID) (domain.ExternalEventDisposition, error) {
	if eventID == "" {
		return domain.ExternalEventDisposition{}, errors.New("external event ID is required")
	}
	var final domain.ExternalEventDisposition
	err := p.Store.Update(ctx, func(tx port.Transaction) error {
		event, err := tx.ExternalEvent(eventID)
		if err != nil {
			return err
		}
		disposition, err := tx.ExternalEventDisposition(eventID)
		if err != nil {
			return err
		}
		if disposition.State.Terminal() {
			final = disposition
			return nil
		}
		now := p.Clock.Now().UTC()
		switch event.Kind {
		case domain.ExternalUserAnswer:
			resultRef, applyErr := p.applyUserAnswer(tx, event, now)
			if applyErr != nil {
				if rejectErr := p.finish(tx, &disposition, domain.ExternalEventRejected, now, "", externalFailureCode(applyErr)); rejectErr != nil {
					return rejectErr
				}
				if err := p.appendDispositionEvent(tx, event, EventExternalEventRejected, disposition.FailureCode, now); err != nil {
					return err
				}
				final = disposition
				return nil
			}
			if err := p.finish(tx, &disposition, domain.ExternalEventApplied, now, resultRef, ""); err != nil {
				return err
			}
			if err := p.appendDispositionEvent(tx, event, EventExternalEventApplied, resultRef, now); err != nil {
				return err
			}
			final = disposition
			return nil
		case domain.ExternalUserMessage, domain.ExternalAvailabilitySignal, domain.ExternalAuthorizedSource:
			resultRef, applyErr := p.applyWake(tx, event, now)
			if applyErr != nil {
				if rejectErr := p.finish(tx, &disposition, domain.ExternalEventRejected, now, "", externalFailureCode(applyErr)); rejectErr != nil {
					return rejectErr
				}
				if err := p.appendDispositionEvent(tx, event, EventExternalEventRejected, disposition.FailureCode, now); err != nil {
					return err
				}
				final = disposition
				return nil
			}
			if resultRef == "" {
				// No matching wait: stimulus is durable evidence only.
				if err := p.finish(tx, &disposition, domain.ExternalEventIgnored, now, "NO_MATCHING_WAIT", ""); err != nil {
					return err
				}
				if err := p.appendDispositionEvent(tx, event, EventExternalEventIgnored, "NO_MATCHING_WAIT", now); err != nil {
					return err
				}
				final = disposition
				return nil
			}
			if err := p.finish(tx, &disposition, domain.ExternalEventApplied, now, resultRef, ""); err != nil {
				return err
			}
			if err := p.appendDispositionEvent(tx, event, EventExternalEventApplied, resultRef, now); err != nil {
				return err
			}
			final = disposition
			return nil
		default:
			if err := p.finish(tx, &disposition, domain.ExternalEventRejected, now, "", "UNSUPPORTED_KIND"); err != nil {
				return err
			}
			if err := p.appendDispositionEvent(tx, event, EventExternalEventRejected, "UNSUPPORTED_KIND", now); err != nil {
				return err
			}
			final = disposition
			return nil
		}
	})
	if err != nil {
		return domain.ExternalEventDisposition{}, err
	}
	return final, nil
}

func (p *ExternalEventProcessor) applyUserAnswer(tx port.Transaction, event domain.ExternalEvent, now time.Time) (string, error) {
	answer, err := DecodeUserAnswerExternalEvent(event)
	if err != nil {
		return "", err
	}
	// Prefer event receipt time for correlation; clock only stamps audits.
	if answer.ReceivedAt.IsZero() {
		answer.ReceivedAt = event.ReceivedAt
	}
	question, err := tx.OperatorQuestion(answer.QuestionID)
	if err != nil {
		return "", err
	}
	if question.MissionID != event.MissionID {
		return "", fmt.Errorf("%w: answer mission disagrees with event envelope", domain.ErrConflict)
	}
	if err := answer.ValidateForQuestion(question); err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrConflict, err)
	}
	answered, err := domain.TransitionOperatorQuestion(question, domain.OperatorQuestionTransition{
		Event:      domain.QuestionEventAnswer,
		OccurredAt: answer.ReceivedAt,
		AnswerID:   answer.ID,
	})
	if err != nil {
		return "", err
	}
	if err := tx.AcceptUserAnswer(answer, answered, question.Revision); err != nil {
		return "", err
	}
	operations, err := tx.Operations(question.MissionRevision)
	if err != nil {
		return "", err
	}
	resumed, err := ResumeQuestionWait(answered, operations)
	if err != nil {
		return "", err
	}
	resumedCount := 0
	for i := range resumed {
		if resumed[i].State != operations[i].State || resumed[i].Reevaluation != operations[i].Reevaluation {
			if err := tx.SaveOperation(resumed[i]); err != nil {
				return "", err
			}
			resumedCount++
		}
	}
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return "", err
	}
	if _, err := tx.AppendEvent(domain.Event{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              domain.EventID(eventID),
		Kind:            EventOperatorQuestionAnswered,
		OccurredAt:      now,
		MissionRevision: question.MissionRevision,
		PayloadRef:      string(answer.ID) + ":" + string(question.ID),
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("answer:%s@%s:resumed=%d", answer.ID, question.ID, resumedCount), nil
}

func (p *ExternalEventProcessor) applyWake(tx port.Transaction, event domain.ExternalEvent, now time.Time) (string, error) {
	// Content is intentionally unused: untrusted stimuli only supply kind,
	// correlation and mission scope for typed waits.
	eventType := wakeEventType(event.Kind)
	if eventType == "" {
		return "", fmt.Errorf("unsupported wake kind %q", event.Kind)
	}
	if event.MissionID == "" {
		// Process-scoped availability without mission cannot target operations.
		return "", nil
	}
	active, err := tx.ActiveMissionRevision(event.MissionID)
	if err != nil {
		return "", err
	}
	operations, err := tx.Operations(active.ID)
	if err != nil {
		return "", err
	}
	resumedCount := 0
	for _, operation := range operations {
		if operation.State != domain.StateWaitingEvent || operation.Reevaluation.EventType != eventType {
			continue
		}
		if event.CorrelationID != "" && operation.Reevaluation.Reference != event.CorrelationID {
			continue
		}
		if event.CorrelationID == "" && operation.Reevaluation.Reference != "" && operation.Reevaluation.Reference != string(event.ID) {
			// Without correlation, only resume waits that either omit reference
			// or explicitly name this durable event id.
			continue
		}
		next, err := domain.Transition(domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}, domain.TransitionInput{Event: domain.EventResume})
		if err != nil {
			return "", fmt.Errorf("resume operation %s: %w", operation.ID, err)
		}
		operation.State, operation.Reevaluation = next.State, next.Reevaluation
		if err := tx.SaveOperation(operation); err != nil {
			return "", err
		}
		resumedCount++
	}
	if resumedCount == 0 {
		return "", nil
	}
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return "", err
	}
	if _, err := tx.AppendEvent(domain.Event{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              domain.EventID(eventID),
		Kind:            EventExternalWakeObserved,
		OccurredAt:      now,
		MissionRevision: active.ID,
		PayloadRef:      string(event.ID) + ":" + eventType + fmt.Sprintf(":resumed=%d", resumedCount),
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("wake:%s:%s:resumed=%d", event.ID, eventType, resumedCount), nil
}

func (p *ExternalEventProcessor) finish(tx port.Transaction, disposition *domain.ExternalEventDisposition, state domain.ExternalEventDispositionState, at time.Time, resultRef, failureCode string) error {
	next := *disposition
	next.State = state
	next.RecordedAt = at.UTC()
	next.ResultRef = resultRef
	next.FailureCode = failureCode
	if err := tx.SaveExternalEventDisposition(next); err != nil {
		return err
	}
	*disposition = next
	return nil
}

func (p *ExternalEventProcessor) appendDispositionEvent(tx port.Transaction, event domain.ExternalEvent, kind, payload string, now time.Time) error {
	eventID, err := p.IDs.NewID("event")
	if err != nil {
		return err
	}
	audit := domain.Event{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.EventID(eventID),
		Kind:          kind,
		OccurredAt:    now,
		PayloadRef:    string(event.ID) + ":" + payload,
	}
	if event.MissionID != "" {
		if active, err := tx.ActiveMissionRevision(event.MissionID); err == nil {
			audit.MissionRevision = active.ID
		}
	}
	_, err = tx.AppendEvent(audit)
	return err
}

func wakeEventType(kind domain.ExternalEventKind) string {
	switch kind {
	case domain.ExternalUserMessage:
		return "user.message"
	case domain.ExternalAvailabilitySignal:
		return "source.available"
	case domain.ExternalAuthorizedSource:
		return "authorized.source"
	default:
		return ""
	}
}

func externalFailureCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrConflict), errors.Is(err, port.ErrConflict):
		return "EXTERNAL_CONFLICT"
	case errors.Is(err, port.ErrNotFound):
		return "TARGET_NOT_FOUND"
	default:
		return "EXTERNAL_INVALID"
	}
}
