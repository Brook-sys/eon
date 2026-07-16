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

const EventQuestionReminderScheduled = "operator.question.reminder.scheduled"

// QuestionReminderProcessor schedules authorized reminder deliveries for
// unanswered questions. It never reopens a terminal question and never creates
// a new canonical question; only outbox rows with stable reminder destinations.
type QuestionReminderProcessor struct {
	Store  port.Store
	Clock  source.Clock
	IDs    source.IDGenerator
	Policy domain.ReminderPolicy
	Routes []QuestionRoute
}

func NewQuestionReminderProcessor(store port.Store, clock source.Clock, ids source.IDGenerator, policy domain.ReminderPolicy, routes []QuestionRoute) (*QuestionReminderProcessor, error) {
	if store == nil || clock == nil || ids == nil {
		return nil, errors.New("question reminder processor requires store, clock, and IDs")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	for i, route := range routes {
		if err := route.Validate(); err != nil {
			return nil, fmt.Errorf("validate reminder route %d: %w", i, err)
		}
	}
	return &QuestionReminderProcessor{Store: store, Clock: clock, IDs: ids, Policy: policy, Routes: append([]QuestionRoute(nil), routes...)}, nil
}

// ScheduleOpenForMission scans non-terminal operator questions for a mission
// and schedules authorized reminders. Returns how many questions produced a
// Due plan (not necessarily how many outbox rows were created).
func (p *QuestionReminderProcessor) ScheduleOpenForMission(ctx context.Context, missionID domain.MissionID, limit int) (int, error) {
	if p == nil {
		return 0, errors.New("question reminder processor is nil")
	}
	if missionID == "" || limit <= 0 {
		return 0, errors.New("reminder mission scan requires mission and positive limit")
	}
	if !p.Policy.Enabled || len(p.Routes) == 0 {
		return 0, nil
	}
	var open []domain.OperatorQuestion
	err := p.Store.View(ctx, func(r port.Reader) error {
		pending, err := r.OperatorQuestions(missionID, domain.OperatorQuestionPending)
		if err != nil {
			return err
		}
		clarifying, err := r.OperatorQuestions(missionID, domain.OperatorQuestionClarificationRequested)
		if err != nil {
			return err
		}
		open = append(pending, clarifying...)
		return nil
	})
	if err != nil {
		return 0, err
	}
	scheduled := 0
	for _, question := range open {
		if scheduled >= limit {
			break
		}
		plan, err := p.ScheduleDue(ctx, question.ID)
		if err != nil {
			return scheduled, err
		}
		if plan.Due {
			scheduled++
		}
	}
	return scheduled, nil
}

// ScheduleDue creates pending reminder deliveries for one question when policy
// and durable observations authorize them. Returns the plan used; zero or more
// deliveries may be created (one per route).
func (p *QuestionReminderProcessor) ScheduleDue(ctx context.Context, questionID domain.OperatorQuestionID) (domain.QuestionReminderPlan, error) {
	now := p.Clock.Now().UTC()
	var plan domain.QuestionReminderPlan
	err := p.Store.Update(ctx, func(tx port.Transaction) error {
		question, err := tx.OperatorQuestion(questionID)
		if err != nil {
			return err
		}
		deliveries, err := tx.QuestionDeliveries(questionID)
		if err != nil {
			return err
		}
		deliveredAt, priorReminders, err := reminderObservations(deliveries)
		if err != nil {
			return err
		}
		plan, err = domain.PlanQuestionReminder(question, deliveredAt, priorReminders, now, p.Policy)
		if err != nil {
			return err
		}
		if !plan.Due {
			return nil
		}
		for _, route := range p.Routes {
			destination := domain.ReminderDestinationRef(route.DestinationRef, plan.ReminderIndex)
			// Idempotent: existing route key means the reminder was already scheduled.
			existing, err := existingDeliveryRoute(tx, question.ID, question.Revision, route.Channel, destination)
			if err != nil {
				return err
			}
			if existing {
				continue
			}
			deliveryID, err := p.IDs.NewID("question_delivery")
			if err != nil {
				return err
			}
			delivery := domain.QuestionDelivery{
				SchemaVersion: domain.SchemaVersionV1, ID: domain.QuestionDeliveryID(deliveryID),
				QuestionID: question.ID, QuestionRevision: question.Revision,
				Channel: route.Channel, DestinationRef: destination,
				Status: domain.QuestionDeliveryPending, MaxAttempts: route.MaxAttempts,
				AvailableAt: plan.AvailableAt, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.CreateQuestionDelivery(delivery); err != nil {
				return err
			}
		}
		eventID, err := p.IDs.NewID("event")
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(eventID),
			Kind: EventQuestionReminderScheduled, OccurredAt: now,
			MissionRevision: question.MissionRevision,
			PayloadRef:      string(question.ID) + ":reminder:" + fmt.Sprint(plan.ReminderIndex),
		})
		return err
	})
	if err != nil {
		return domain.QuestionReminderPlan{}, err
	}
	return plan, nil
}

func reminderObservations(deliveries []domain.QuestionDelivery) (time.Time, uint32, error) {
	var firstDelivered time.Time
	var reminders uint32
	for _, delivery := range deliveries {
		if delivery.Status != domain.QuestionDeliveryDelivered {
			continue
		}
		if isReminderDestination(delivery.DestinationRef) {
			reminders++
			continue
		}
		if firstDelivered.IsZero() || delivery.UpdatedAt.Before(firstDelivered) {
			firstDelivered = delivery.UpdatedAt
		}
	}
	return firstDelivered, reminders, nil
}

func isReminderDestination(destination string) bool {
	return len(destination) > 0 && (containsReminderMarker(destination))
}

func containsReminderMarker(destination string) bool {
	const marker = "#reminder:"
	for i := 0; i+len(marker) <= len(destination); i++ {
		if destination[i:i+len(marker)] == marker {
			return true
		}
	}
	return false
}

func existingDeliveryRoute(tx port.Transaction, questionID domain.OperatorQuestionID, revision uint64, channel, destination string) (bool, error) {
	deliveries, err := tx.QuestionDeliveries(questionID)
	if err != nil {
		return false, err
	}
	for _, delivery := range deliveries {
		if delivery.QuestionRevision == revision && delivery.Channel == channel && delivery.DestinationRef == destination {
			return true, nil
		}
	}
	return false, nil
}
