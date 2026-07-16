package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func installGateMission(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "test question gate", Purpose: "test question gate", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now.Add(-time.Hour),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.AppendMissionRevision(mission)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGateProcessorAdmitsQuestionDeliveryAndAuditAtomically(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	installGateMission(t, store, now)
	processor, err := NewQuestionGateProcessor(store, source.NewManualClock(now), source.NewSequenceIDGenerator(1), gatePolicy(), "default@1", []QuestionRoute{{Channel: "dashboard", DestinationRef: "operator_primary", MaxAttempts: 3}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := gateProposal(now)
	decision, err := processor.Process(context.Background(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != domain.PersistedQuestionAdmit {
		t.Fatalf("decision = %#v", decision)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		question, err := r.OperatorQuestion(proposal.Question.ID)
		if err != nil {
			return err
		}
		if question.ID != proposal.Question.ID {
			t.Fatalf("question = %#v", question)
		}
		deliveries, err := r.QuestionDeliveries(question.ID)
		if err != nil {
			return err
		}
		if len(deliveries) != 1 || deliveries[0].Channel != "dashboard" || deliveries[0].Status != domain.QuestionDeliveryPending {
			t.Fatalf("deliveries = %#v", deliveries)
		}
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].Kind != EventQuestionGateAdmitted {
			t.Fatalf("events = %#v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	replayed, err := processor.Process(context.Background(), proposal)
	if err != nil || replayed.ID != decision.ID {
		t.Fatalf("replay = %#v, err = %v", replayed, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		deliveries, _ := r.QuestionDeliveries(proposal.Question.ID)
		events, _ := r.Events(0, 10)
		if len(deliveries) != 1 || len(events) != 1 {
			t.Fatalf("replay duplicated effects: deliveries=%d events=%d", len(deliveries), len(events))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGateProcessorRollsBackDecisionQuestionAndOutboxTogether(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	installGateMission(t, store, now)
	route := QuestionRoute{Channel: "dashboard", DestinationRef: "operator_primary", MaxAttempts: 3}
	processor, err := NewQuestionGateProcessor(store, source.NewManualClock(now), source.NewSequenceIDGenerator(10), gatePolicy(), "default@1", []QuestionRoute{route, route})
	if err != nil {
		t.Fatal(err)
	}
	proposal := gateProposal(now)
	if _, err := processor.Process(context.Background(), proposal); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("process error = %v", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		if _, err := r.QuestionGateDecisionByQuestion(proposal.Question.ID); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("gate decision survived rollback: %v", err)
		}
		if _, err := r.OperatorQuestion(proposal.Question.ID); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("question survived rollback: %v", err)
		}
		deliveries, _ := r.QuestionDeliveries(proposal.Question.ID)
		events, _ := r.Events(0, 10)
		if len(deliveries) != 0 || len(events) != 0 {
			t.Fatalf("effects survived rollback: deliveries=%d events=%d", len(deliveries), len(events))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGateProcessorDigestDefersOutboxAvailability(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	installGateMission(t, store, now)
	policy := gatePolicy()
	policy.Digest = domain.DigestPolicy{Hold: 90 * time.Minute, MaxItems: 4, MinPriorityImmediate: 80}
	processor, err := NewQuestionGateProcessor(store, source.NewManualClock(now), source.NewSequenceIDGenerator(30), policy, "default@1", []QuestionRoute{{Channel: "dashboard", DestinationRef: "operator_primary", MaxAttempts: 3}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := gateProposal(now)
	decision, err := processor.Process(context.Background(), proposal)
	if err != nil || decision.Decision != domain.PersistedQuestionAdmit {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		deliveries, err := r.QuestionDeliveries(proposal.Question.ID)
		if err != nil {
			return err
		}
		if len(deliveries) != 1 || !deliveries[0].AvailableAt.Equal(now.Add(90*time.Minute)) {
			t.Fatalf("digest deliveries = %#v", deliveries)
		}
		if deliveries[0].Due(now) {
			t.Fatal("digest delivery should not be due immediately")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGateProcessorPersistsSuppressionWithoutCanonicalQuestion(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	installGateMission(t, store, now)
	processor, err := NewQuestionGateProcessor(store, source.NewManualClock(now), source.NewSequenceIDGenerator(20), gatePolicy(), "default@1", nil)
	if err != nil {
		t.Fatal(err)
	}
	proposal := gateProposal(now)
	proposal.Justification.HasSafeDefault = true
	proposal.Justification.DefaultReversible = true
	decision, err := processor.Process(context.Background(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != domain.PersistedQuestionSuppress || decision.Reason != domain.PersistedQuestionGateSafeDefault {
		t.Fatalf("decision = %#v", decision)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		if _, err := r.OperatorQuestion(proposal.Question.ID); !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("suppressed question persisted: %v", err)
		}
		deliveries, err := r.QuestionDeliveries(proposal.Question.ID)
		if err != nil || len(deliveries) != 0 {
			t.Fatalf("suppressed deliveries = %#v, err = %v", deliveries, err)
		}
		events, _ := r.Events(0, 10)
		if len(events) != 1 || events[0].Kind != EventQuestionGateSuppressed {
			t.Fatalf("events = %#v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
