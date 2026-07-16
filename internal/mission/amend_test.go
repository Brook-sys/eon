package mission

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/storage/sqlite"
	"path/filepath"
)

func seedMissionWithOpenWork(t *testing.T, store port.Store, now time.Time) (domain.MissionRevision, domain.Operation, domain.Inquiry, domain.WorkOpportunity) {
	t.Helper()
	mission := domain.MissionRevision{
		SchemaVersion: 1, ID: "mission_revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "Investigate epistemic runtimes.", Purpose: "Build cited knowledge",
		Domains: []string{"runtime", "knowledge"}, Policies: []string{"cite"},
		Budget: domain.Budget{ModelCalls: 10, Tokens: 8000, Bytes: 1024, Attempts: 2, Duration: time.Hour},
		Status: domain.MissionActive, Provenance: "user:seed", AcceptedAt: now,
	}
	spec := domain.OperationSpec{
		SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "input", OutputSchema: "output",
		Budget:          domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1},
		MaxOutputTokens: 100, SafetyMargin: 10, Validators: []string{"schema"},
		RetryPolicy: "none", FallbackPolicy: "none", MaximumAuthority: domain.AuthorityProposeOnly,
	}
	question := domain.Question{
		SchemaVersion: 1, ID: "question_1", MissionRevision: mission.ID,
		Text: "What is supported?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
	}
	candidate := domain.InquiryCandidate{
		SchemaVersion: 1, ID: "candidate_1", MissionRevision: mission.ID, QuestionID: question.ID,
		DerivedFrom: []string{"mission"}, ExpectedProgress: "reduce uncertainty", Novelty: "new",
		EstimatedCost: domain.Budget{ModelCalls: 1, Tokens: 500, Attempts: 1}, Risk: domain.RiskLow,
		SourcePlan: []string{"fixtures"}, AnswerCondition: "evidence", StopCondition: "done",
		ReviewAfter: now.Add(24 * time.Hour),
	}
	inquiry := domain.Inquiry{
		SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: mission.ID,
		QuestionID: question.ID, AdmissionReason: "priority", Budget: domain.Budget{ModelCalls: 2, Tokens: 1000, Attempts: 2},
		StopCondition: "done", State: domain.StateReady,
		Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	operation := domain.Operation{
		SchemaVersion: 1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: mission.ID,
		SpecID: spec.ID, ReadSet: []string{"source_1"}, InputRefs: []string{"ctx"}, ExpectedOutput: "proposal",
		IdempotencyKey: "idem_1", State: domain.StateReady,
		Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	opp := domain.WorkOpportunity{
		SchemaVersion: 1, ID: "work_opportunity_1", MissionRevision: mission.ID,
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen,
		Title: "gap", Origin: "seed", ExpectedGain: "fill gap",
		Novelty: "new", StopCondition: "done", DedupSignature: "gap:seed",
		EstimatedCost: domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1},
		Risk:          domain.RiskLow, Priority: 10, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
			return err
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		if err := tx.CreateOperation(operation); err != nil {
			return err
		}
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return mission, operation, inquiry, opp
}

func TestAcceptorInstallsRevisionCancelsAgendaAndPreservesPrevious(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	previous, _, _, _ := seedMissionWithOpenWork(t, store, now)

	acceptor := Acceptor{
		Store: store,
		Clock: source.NewManualClock(now.Add(time.Hour)),
		IDs:   source.NewSequenceIDGenerator(10),
	}
	amendment := domain.UserAmendment{
		SchemaVersion: 1, MissionID: previous.MissionID, BaseRevision: previous.Revision, CandidateRevision: previous.Revision + 1,
		OriginalText: "Investigate epistemic runtimes and continuity.",
		Purpose:      "Build cited knowledge with durable continuity",
		Domains:      []string{"runtime", "knowledge", "continuity"},
		Policies:     []string{"cite"},
		Budget:       domain.Budget{ModelCalls: 4, Tokens: 4000, Bytes: 1024, Attempts: 2, Duration: time.Hour},
		Status:       domain.MissionActive,
		Reason:       "expand continuity",
	}
	result, err := acceptor.Accept(context.Background(), amendment, "user:amend")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if result.Accepted.Revision != 2 || result.Accepted.ID == previous.ID {
		t.Fatalf("accepted = %#v", result.Accepted)
	}
	if result.Report.PreviousRevision != previous.ID || result.Report.NewRevision != result.Accepted.ID {
		t.Fatalf("report = %#v", result.Report)
	}
	if len(result.Report.CancelledOperations) != 1 || result.Report.CancelledOperations[0] != "operation_1" {
		t.Fatalf("cancelled operations = %#v", result.Report.CancelledOperations)
	}
	if len(result.Report.CancelledInquiries) != 1 || result.Report.CancelledInquiries[0] != "inquiry_1" {
		t.Fatalf("cancelled inquiries = %#v", result.Report.CancelledInquiries)
	}
	if len(result.Report.AbandonedOpportunities) != 1 {
		t.Fatalf("abandoned opportunities = %#v", result.Report.AbandonedOpportunities)
	}
	if result.Diff.Empty || !result.Impact.RequiresAcceptance {
		t.Fatalf("diff/impact = %#v %#v", result.Diff, result.Impact)
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(previous.MissionID)
		if err != nil {
			return err
		}
		if active.ID != result.Accepted.ID || active.OriginalText != amendment.OriginalText {
			t.Fatalf("active = %#v", active)
		}
		old, err := r.MissionRevision(previous.ID)
		if err != nil {
			return err
		}
		if old.OriginalText != previous.OriginalText || old.Revision != 1 {
			t.Fatalf("previous mutated: %#v", old)
		}
		op, err := r.Operation("operation_1")
		if err != nil {
			return err
		}
		if op.State != domain.StateCancelled {
			t.Fatalf("operation state = %s", op.State)
		}
		inquiry, err := r.Inquiry("inquiry_1")
		if err != nil {
			return err
		}
		if inquiry.State != domain.StateCancelled {
			t.Fatalf("inquiry state = %s", inquiry.State)
		}
		opp, err := r.WorkOpportunity("work_opportunity_1")
		if err != nil {
			return err
		}
		if opp.Status != domain.OpportunityAbandoned {
			t.Fatalf("opportunity status = %s", opp.Status)
		}
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		kinds := map[string]int{}
		for _, e := range events {
			kinds[e.Kind]++
		}
		for _, need := range []string{EventMissionAmendmentAccepted, EventMissionAgendaReconciled, EventMissionOperationCancelled, EventMissionInquiryCancelled, EventMissionOpportunityAbandoned} {
			if kinds[need] == 0 {
				t.Fatalf("missing event %s in %#v", need, kinds)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptorRejectsNoopWithoutMutation(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
	previous, _, _, _ := seedMissionWithOpenWork(t, store, now)
	acceptor := Acceptor{Store: store, Clock: source.NewManualClock(now), IDs: source.NewSequenceIDGenerator(1)}
	noop := domain.UserAmendment{
		SchemaVersion: 1, MissionID: previous.MissionID, BaseRevision: previous.Revision, CandidateRevision: previous.Revision + 1,
		OriginalText: previous.OriginalText, Purpose: previous.Purpose,
		Domains: append([]string(nil), previous.Domains...), Policies: append([]string(nil), previous.Policies...),
		Budget: previous.Budget, Status: previous.Status, Reason: "noop",
	}
	if _, err := acceptor.Accept(context.Background(), noop, "user"); err == nil {
		t.Fatal("noop amendment accepted")
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(previous.MissionID)
		if err != nil {
			return err
		}
		if active.ID != previous.ID {
			t.Fatalf("active changed on noop: %#v", active)
		}
		op, err := r.Operation("operation_1")
		if err != nil {
			return err
		}
		if op.State != domain.StateReady {
			t.Fatalf("operation mutated: %s", op.State)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptorWorksOnSQLiteDurableStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 7, 17, 14, 0, 0, 0, time.UTC)
	previous, _, _, _ := seedMissionWithOpenWork(t, store, now)
	acceptor := Acceptor{Store: store, Clock: source.NewManualClock(now.Add(time.Minute)), IDs: source.NewSequenceIDGenerator(20)}
	amendment := domain.UserAmendment{
		SchemaVersion: 1, MissionID: previous.MissionID, BaseRevision: previous.Revision, CandidateRevision: 2,
		OriginalText: previous.OriginalText + " Continuity.",
		Purpose:      previous.Purpose, Domains: previous.Domains, Policies: previous.Policies,
		Budget: previous.Budget, Status: domain.MissionActive, Reason: "text expansion",
	}
	result, err := acceptor.Accept(context.Background(), amendment, "user:sqlite")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.View(context.Background(), func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(previous.MissionID)
		if err != nil {
			return err
		}
		if active.ID != result.Accepted.ID {
			t.Fatalf("active after reopen = %#v", active)
		}
		op, err := r.Operation("operation_1")
		if err != nil {
			return err
		}
		if op.State != domain.StateCancelled {
			t.Fatalf("operation after reopen = %s", op.State)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
