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
	scheduler := Scheduler{Store: store, MemoryStore: store, Clock: clock, Strategies: strategies}

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
	scheduler := Scheduler{Store: store, MemoryStore: store, Clock: clock, Strategies: []ContinuityStrategy{
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
	scheduler := Scheduler{Store: store, MemoryStore: store, Clock: clock}

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

func TestSchedulerRegistryExpandPersistsDiagnosisOnBlock(t *testing.T) {
	now := time.Date(2026, 7, 16, 7, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	reg := NewStrategyRegistry()
	if err := reg.Register(StrategyDescriptor{
		Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v1", Priority: 20,
	}, continuityStrategy{name: "gap_scan", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
		return ContinuityResult{}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(StrategyDescriptor{
		Name: "integrity_audit", Family: domain.FamilyIntegrityAudit, Version: "v1", Priority: 10, LocalOnly: true,
	}, continuityStrategy{name: "integrity_audit", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
		return ContinuityResult{}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	scheduler := Scheduler{Store: store, MemoryStore: store, Clock: clock, Registry: reg, IDs: source.NewSequenceIDGenerator(1)}
	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionContinuityBlocked || decision.Action != domain.ContinuityDiagnose || decision.DiagnosisID == "" {
		t.Fatalf("decision = %+v", decision)
	}
	if len(decision.StrategiesTried) != 2 || decision.StrategiesTried[0] != "gap_scan@v1" || decision.StrategiesTried[1] != "integrity_audit@v1" {
		t.Fatalf("strategies tried = %#v", decision.StrategiesTried)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		diag, err := r.LatestContinuityDiagnosis("revision_1")
		if err != nil {
			return err
		}
		if diag.ID != decision.DiagnosisID || diag.ReadyCount != 0 || diag.PolicyVersion == "" {
			t.Fatalf("diagnosis = %+v", diag)
		}
		opps, err := r.WorkOpportunities("revision_1", "")
		if err != nil {
			return err
		}
		if len(opps) != 0 {
			t.Fatalf("unexpected opportunities: %#v", opps)
		}
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		found := false
		for _, event := range events {
			if event.Kind == domain.EventContinuityBlocked {
				found = true
			}
		}
		if !found {
			t.Fatal("missing continuity.blocked event")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkOpportunityPersistenceAndChildFanout(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	store := memory.New()
	seedMission(t, store)
	root := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_root", MissionRevision: "revision_1",
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "cover gaps", Origin: "mission",
		ExpectedGain: "new inquiries", Novelty: "uncovered scopes", StopCondition: "coverage target",
		DedupSignature: "gap:root", Depth: 0, EstimatedCost: domain.Budget{Tokens: 10}, Risk: domain.RiskLow,
		Priority: 10, CreatedAt: now, UpdatedAt: now,
	}
	child, err := root.DeriveChild("opp_child", "define term", "decompose:root", "definition", "undefined term", "definition accepted", "gap:term", domain.RiskLow, 8, now.Add(time.Minute), domain.Budget{Tokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateWorkOpportunity(root); err != nil {
			return err
		}
		return tx.CreateWorkOpportunity(child)
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	dup := root
	dup.ID = "opp_dup"
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(dup)
	}); err == nil {
		t.Fatal("expected dedup conflict")
	}
	admitted := root
	admitted.Status = domain.OpportunityAdmitted
	admitted.AdmittedInquiryID = "inquiry_1"
	admitted.UpdatedAt = now.Add(2 * time.Minute)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveWorkOpportunity(admitted)
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	blob, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := memory.NewFromBinary(blob)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.View(context.Background(), func(r port.Reader) error {
		got, err := r.WorkOpportunity("opp_child")
		if err != nil || got.ParentID != "opp_root" || got.Depth != 1 {
			t.Fatalf("child = %+v err=%v", got, err)
		}
		open, err := r.WorkOpportunities("revision_1", domain.OpportunityOpen)
		if err != nil || len(open) != 1 || open[0].ID != "opp_child" {
			t.Fatalf("open = %#v err=%v", open, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
	scheduler := Scheduler{Store: store, MemoryStore: store, Clock: clock}
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
