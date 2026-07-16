package kernel

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

func TestQuestionGateDecisionAndOutboxSurviveSQLiteReopen(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "question-gate.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	installGateMission(t, store, now)
	processor, err := NewQuestionGateProcessor(store, source.NewManualClock(now), source.NewSequenceIDGenerator(100), gatePolicy(), "default@1", []QuestionRoute{{Channel: "telegram", DestinationRef: "operator_primary", MaxAttempts: 3}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := gateProposal(now)
	decision, err := processor.Process(context.Background(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.QuestionGateDecision(decision.ID)
		if err != nil {
			return err
		}
		if got.Decision != domain.PersistedQuestionAdmit || got.PolicyVersion != "default@1" {
			t.Fatalf("decision after reopen = %#v", got)
		}
		deliveries, err := r.QuestionDeliveries(proposal.Question.ID)
		if err != nil {
			return err
		}
		if len(deliveries) != 1 || deliveries[0].Status != domain.QuestionDeliveryPending {
			t.Fatalf("deliveries after reopen = %#v", deliveries)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewQuestionGateProcessor(store, source.NewManualClock(now.Add(time.Hour)), source.NewSequenceIDGenerator(200), gatePolicy(), "default@1", []QuestionRoute{{Channel: "telegram", DestinationRef: "operator_primary", MaxAttempts: 3}})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Process(context.Background(), proposal)
	if err != nil || replayed.ID != decision.ID {
		t.Fatalf("replayed decision = %#v, err = %v", replayed, err)
	}
}
