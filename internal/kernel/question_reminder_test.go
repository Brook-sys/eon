package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func seedDeliveredQuestion(t *testing.T, store port.Store, question domain.OperatorQuestion, deliveredAt time.Time) {
	t.Helper()
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		delivery := domain.QuestionDelivery{
			SchemaVersion: domain.SchemaVersionV1, ID: "delivery_primary",
			QuestionID: question.ID, QuestionRevision: question.Revision,
			Channel: "dashboard", DestinationRef: "operator_primary",
			Status: domain.QuestionDeliveryPending, MaxAttempts: 3,
			AvailableAt: deliveredAt, CreatedAt: deliveredAt, UpdatedAt: deliveredAt,
		}
		if err := tx.CreateQuestionDelivery(delivery); err != nil {
			return err
		}
		leaseStart := deliveredAt
		leaseUntil := deliveredAt.Add(time.Minute)
		leased, err := domain.LeaseQuestionDelivery(delivery, "worker", leaseStart, leaseUntil)
		if err != nil {
			return err
		}
		if err := tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt); err != nil {
			return err
		}
		done, err := domain.CompleteQuestionDelivery(leased, "worker", "msg-1", leaseStart.Add(time.Second))
		if err != nil {
			return err
		}
		return tx.SaveQuestionDelivery(done, leased.Status, leased.Attempt)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionReminderProcessorSchedulesAndStops(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	installGateMission(t, store, now)
	question := gateProposal(now).Question
	question.CreatedAt = now.Add(-3 * time.Hour)
	question.ExpiresAt = now.Add(24 * time.Hour)
	seedDeliveredQuestion(t, store, question, now.Add(-3*time.Hour))

	processor, err := NewQuestionReminderProcessor(
		store, source.NewManualClock(now), source.NewSequenceIDGenerator(1),
		domain.ReminderPolicy{Enabled: true, MaxCount: 1, FirstAfter: 2 * time.Hour, Interval: time.Hour},
		[]QuestionRoute{{Channel: "dashboard", DestinationRef: "operator_primary", MaxAttempts: 2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := processor.ScheduleDue(context.Background(), question.ID)
	if err != nil || !plan.Due || plan.ReminderIndex != 1 {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		deliveries, err := r.QuestionDeliveries(question.ID)
		if err != nil {
			return err
		}
		if len(deliveries) != 2 {
			t.Fatalf("deliveries = %#v", deliveries)
		}
		found := false
		for _, delivery := range deliveries {
			if delivery.DestinationRef == domain.ReminderDestinationRef("operator_primary", 1) {
				found = true
				if delivery.Status != domain.QuestionDeliveryPending {
					t.Fatalf("reminder status = %s", delivery.Status)
				}
			}
		}
		if !found {
			t.Fatalf("reminder delivery missing: %#v", deliveries)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	replay, err := processor.ScheduleDue(context.Background(), question.ID)
	if err != nil || !replay.Due {
		t.Fatalf("replay plan = %+v err=%v", replay, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		deliveries, err := r.QuestionDeliveries(question.ID)
		if err != nil {
			return err
		}
		if len(deliveries) != 2 {
			t.Fatalf("replay duplicated deliveries: %d", len(deliveries))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Deliver the reminder so MaxCount=1 stops further planning.
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		deliveries, err := tx.QuestionDeliveries(question.ID)
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			if delivery.DestinationRef != domain.ReminderDestinationRef("operator_primary", 1) {
				continue
			}
			leaseStart := now.Add(time.Minute)
			leased, err := domain.LeaseQuestionDelivery(delivery, "worker", leaseStart, leaseStart.Add(time.Minute))
			if err != nil {
				return err
			}
			if err := tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt); err != nil {
				return err
			}
			done, err := domain.CompleteQuestionDelivery(leased, "worker", "msg-reminder", leaseStart.Add(time.Second))
			if err != nil {
				return err
			}
			return tx.SaveQuestionDelivery(done, leased.Status, leased.Attempt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	laterClock := source.NewManualClock(now.Add(10 * time.Hour))
	stoppedProcessor, err := NewQuestionReminderProcessor(store, laterClock, source.NewSequenceIDGenerator(50), domain.ReminderPolicy{Enabled: true, MaxCount: 1, FirstAfter: 2 * time.Hour, Interval: time.Hour}, []QuestionRoute{{Channel: "dashboard", DestinationRef: "operator_primary", MaxAttempts: 2}})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := stoppedProcessor.ScheduleDue(context.Background(), question.ID)
	if err != nil || stopped.Due || stopped.StopReason != "MAX_REMINDERS" {
		t.Fatalf("stopped = %+v err=%v", stopped, err)
	}
}

func TestQuestionReminderProcessorDisabledCreatesNothing(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	installGateMission(t, store, now)
	question := gateProposal(now).Question
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateOperatorQuestion(question)
	}); err != nil {
		t.Fatal(err)
	}
	processor, err := NewQuestionReminderProcessor(store, source.NewManualClock(now), source.NewSequenceIDGenerator(1), domain.ReminderPolicy{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := processor.ScheduleDue(context.Background(), question.ID)
	if err != nil || plan.StopReason != "REMINDERS_DISABLED" {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
}
