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

type continuityStrategy struct {
	name string
	run  func(context.Context, domain.MissionRevisionID) (ContinuityResult, error)
}

func (s continuityStrategy) Name() string { return s.name }
func (s continuityStrategy) Replenish(ctx context.Context, mission domain.MissionRevisionID) (ContinuityResult, error) {
	return s.run(ctx, mission)
}

func TestSchedulerReportsContinuityBlockedAfterTryingEveryStrategy(t *testing.T) {
	start := time.Date(2026, 7, 15, 15, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(start)
	store := memory.New()
	seedMission(t, store)
	var calls []string
	strategies := []ContinuityStrategy{
		continuityStrategy{name: "gap-scan", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
			calls = append(calls, "gap-scan")
			return ContinuityResult{}, nil
		}},
		continuityStrategy{name: "integrity-audit", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
			calls = append(calls, "integrity-audit")
			return ContinuityResult{Changed: true}, nil
		}},
	}
	scheduler := Scheduler{Store: store, Clock: clock, Strategies: strategies}

	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionContinuityBlocked {
		t.Fatalf("decision = %+v, want continuity blocked", decision)
	}
	if got, want := len(calls), 2; got != want {
		t.Fatalf("strategy calls = %d, want %d", got, want)
	}
	if len(decision.StrategiesTried) != 2 || decision.ContinuityFailure == "" {
		t.Fatalf("incomplete continuity diagnosis: %+v", decision)
	}
}

func TestSchedulerDispatchesWorkAdmittedByAnotherContinuityFamily(t *testing.T) {
	now := time.Date(2026, 7, 15, 15, 50, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	var fallbackCalled bool
	scheduler := Scheduler{Store: store, Clock: clock, Strategies: []ContinuityStrategy{
		continuityStrategy{name: "gap-scan", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
			return ContinuityResult{}, nil
		}},
		continuityStrategy{name: "artifact-refresh", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
			seedAgendaRecords(t, store, now)
			return ContinuityResult{Admitted: 2, Changed: true}, nil
		}},
		continuityStrategy{name: "unused-fallback", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
			fallbackCalled = true
			return ContinuityResult{}, nil
		}},
	}}

	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionDispatch || decision.Operation != "operation_a" {
		t.Fatalf("decision = %+v, want operation_a dispatch", decision)
	}
	if fallbackCalled {
		t.Fatal("scheduler continued to later strategy after work became ready")
	}
}

func TestSchedulerResumesDueOperationOnceAndSelectsDeterministically(t *testing.T) {
	now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedAgenda(t, store, now)
	scheduler := Scheduler{Store: store, Clock: clock}

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

func TestSchedulerPauseBlocksNewDispatchButStillResumesLocalWaits(t *testing.T) {
	now := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedAgenda(t, store, now)
	revision := uint64(1)
	control := domain.DefaultControlState(now)
	control.Missions = map[domain.MissionID]domain.MissionControl{
		"mission_1": {
			MissionID: "mission_1", Mode: domain.MissionDispatchPaused, RevisionAtChange: revision,
			Reason: "hold", LastCommandID: "cmd_pause", UpdatedAt: now,
		},
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveControlState(control, 0)
	}); err != nil {
		t.Fatal(err)
	}
	scheduler := Scheduler{Store: store, Clock: clock}
	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionContinuityBlocked {
		t.Fatalf("decision = %+v, want continuity blocked while paused", decision)
	}
	var resumed domain.Operation
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		resumed, err = r.Operation("operation_b")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if resumed.State != domain.StateReady {
		t.Fatalf("local wait was not resumed under pause: %+v", resumed)
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
	seedAgendaRecords(t, store, now)
}

func seedAgendaRecords(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	q := domain.Question{SchemaVersion: 1, ID: "question_1", MissionRevision: "revision_1", Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence"}
	c := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_1", MissionRevision: "revision_1", QuestionID: q.ID, DerivedFrom: []string{"gap"}, ExpectedProgress: "answer", Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "evidence", StopCondition: "done", ReviewAfter: now.Add(time.Hour)}
	i := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_1", CandidateID: c.ID, MissionRevision: "revision_1", QuestionID: q.ID, AdmissionReason: "test", StopCondition: "done", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	spec := domain.OperationSpec{SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "text", OutputSchema: "json", Budget: domain.Budget{Tokens: 100}, MaxOutputTokens: 20, SafetyMargin: 10, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "another-continuity-strategy", MaximumAuthority: domain.AuthorityProposeOnly}
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
