package kernel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

type countingReplenisher struct{ calls atomic.Int32 }

func (r *countingReplenisher) Replenish(context.Context, domain.MissionRevisionID) (bool, error) {
	r.calls.Add(1)
	return true, nil
}

func TestSchedulerPersistsRestAndBlocksUntilVirtualDeadline(t *testing.T) {
	start := time.Date(2026, 7, 15, 15, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(start)
	store := memory.New()
	seedMission(t, store)
	replenisher := &countingReplenisher{}
	scheduler := Scheduler{Store: store, Clock: clock, Replenisher: replenisher, MaxReplenishment: 3, RestInterval: time.Hour}

	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionRest || replenisher.calls.Load() != 3 {
		t.Fatalf("decision = %+v, replenishment calls = %d", decision, replenisher.calls.Load())
	}

	done := make(chan error, 1)
	go func() { done <- scheduler.Wait(context.Background(), decision.Rest) }()
	assertStillWaiting(t, done)
	if err := clock.Advance(59 * time.Minute); err != nil {
		t.Fatal(err)
	}
	assertStillWaiting(t, done)
	if err := clock.Advance(time.Minute); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not wake at virtual deadline")
	}

	if got := persistedRest(t, store); got.Active || got.WokenAt == nil || !got.ReevaluationIsZero() {
		t.Fatalf("persisted rest after wake = %+v", got)
	}
}

func TestSchedulerResumesDueOperationOnceAndSelectsDeterministically(t *testing.T) {
	now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedAgenda(t, store, now)
	scheduler := Scheduler{Store: store, Clock: clock, MaxReplenishment: 1, RestInterval: time.Hour}

	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionDispatch || decision.Operation != "operation_a" {
		t.Fatalf("decision = %+v, want operation_a", decision)
	}
	var resumed domain.Operation
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		resumed, err = r.Operation("operation_b")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if resumed.State != domain.StateReady || resumed.Reevaluation.Kind != domain.ReevaluateReady {
		t.Fatalf("due operation was not resumed: %+v", resumed)
	}
}

func assertStillWaiting(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("wait returned before virtual deadline: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func seedMission(t *testing.T, store port.Store) {
	t.Helper()
	revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "research", Purpose: "learn", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendMissionRevision(revision) }); err != nil {
		t.Fatalf("seed mission: %v", err)
	}
}

func seedAgenda(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	seedMission(t, store)
	q := domain.Question{SchemaVersion: 1, ID: "question_1", MissionRevision: "revision_1", Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence"}
	c := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_1", MissionRevision: "revision_1", QuestionID: q.ID, DerivedFrom: []string{"gap"}, ExpectedProgress: "answer", Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "evidence", StopCondition: "done", ReviewAfter: now.Add(time.Hour)}
	i := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_1", CandidateID: c.ID, MissionRevision: "revision_1", QuestionID: q.ID, AdmissionReason: "test", StopCondition: "done", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	spec := domain.OperationSpec{SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "text", OutputSchema: "json", Budget: domain.Budget{Tokens: 100}, MaxOutputTokens: 20, SafetyMargin: 10, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "rest", MaximumAuthority: domain.AuthorityProposeOnly}
	due := now.Add(-time.Minute)
	operations := []domain.Operation{
		{SchemaVersion: 1, ID: "operation_b", InquiryID: i.ID, MissionRevision: "revision_1", SpecID: spec.ID, ExpectedOutput: "changeset", IdempotencyKey: "idem_b", State: domain.StateWaitingTime, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateNotBefore, NotBefore: &due}},
		{SchemaVersion: 1, ID: "operation_a", InquiryID: i.ID, MissionRevision: "revision_1", SpecID: spec.ID, ExpectedOutput: "changeset", IdempotencyKey: "idem_a", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}},
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		if err := tx.CreateQuestion(q); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(c); err != nil {
			return err
		}
		if err := tx.CreateInquiry(i); err != nil {
			return err
		}
		for _, operation := range operations {
			if err := tx.CreateOperation(operation); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed agenda: %v", err)
	}
}

func persistedRest(t *testing.T, store port.Store) domain.Rest {
	t.Helper()
	var rest domain.Rest
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		rest, err = r.Rest("revision_1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return rest
}
