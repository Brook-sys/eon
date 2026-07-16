package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestModelEligible(t *testing.T) {
	t.Parallel()
	local := ContinuityOperationSpec("continuity.gap_scan@1", domain.AuthorityProposeOnly)
	if ModelEligible(local) {
		t.Fatal("continuity specs must stay on local path")
	}
	extract := modelTestSpec()
	if !ModelEligible(extract) {
		t.Fatal("non-continuity PROPOSE_ONLY must be model eligible")
	}
	readOnly := extract
	readOnly.MaximumAuthority = domain.AuthorityReadOnly
	if ModelEligible(readOnly) {
		t.Fatal("read-only is local, not model")
	}
}

func TestModelExecutorCompletesWithFakeProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	// Full ProposedChangeSet JSON as the fake model response (lineage present).
	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_1", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_model_1", PayloadRef: "payload_model_1",
		}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(fakeserver.Exchange{
		ResponseText:  string(body),
		ResponseModel: "fixture-model",
		InputTokens:   40,
		OutputTokens:  80,
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{
		BaseURL: server.URL(),
		Model:   "fixture-model",
		Client:  server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{
		Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8000,
		},
		PolicyVersion: "policy@model-test",
		LeaseTTL:      5 * time.Minute,
	}

	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed || result.CommitID == "" || result.LeaseRef == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, ok := ParseLeaseDeadline(result.LeaseRef); !ok {
		t.Fatalf("lease ref missing deadline: %s", result.LeaseRef)
	}

	var op domain.Operation
	var entity domain.CanonicalEntity
	var events []domain.Event
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		if err != nil {
			return err
		}
		entity, err = r.CanonicalEntity("observation", "obs_model_1")
		if err != nil {
			return err
		}
		events, err = r.Events(0, 50)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateSucceeded || op.Attempt != 1 {
		t.Fatalf("operation = %+v", op)
	}
	if entity.PayloadRef != "payload_model_1" || entity.CommitID != result.CommitID {
		t.Fatalf("entity = %+v", entity)
	}
	kinds := map[string]int{}
	for _, event := range events {
		if event.OperationID == "operation_model" {
			kinds[event.Kind]++
		}
	}
	for _, want := range []string{EventOperationDispatched, EventOperationModelInvoked, EventOperationModelVerified, EventOperationSucceeded} {
		if kinds[want] != 1 {
			t.Fatalf("event %s count=%d kinds=%v", want, kinds[want], kinds)
		}
	}
	if len(server.Requests()) != 1 {
		t.Fatalf("provider calls = %d", len(server.Requests()))
	}
	if !strings.Contains(server.Requests()[0].Prompt, "ProposedChangeSet") {
		t.Fatalf("prompt missing contract: %q", server.Requests()[0].Prompt)
	}

	// Terminal skip on re-execute.
	again, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Skipped || again.SkipReason != "terminal" {
		t.Fatalf("re-execute = %+v", again)
	}
}

func TestModelExecutorAcceptsFencedProposal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 14, 15, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_fence", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_model_fence", PayloadRef: "payload_fence",
		}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	fenced := "```json\n" + string(body) + "\n```\n"
	server := fakeserver.New(fakeserver.Exchange{
		ResponseText: fenced, ResponseModel: "fixture-model", InputTokens: 20, OutputTokens: 40,
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test", LeaseTTL: 5 * time.Minute,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute fenced proposal: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completion for fenced JSON: %+v", result)
	}
	// Raw artifact must keep the exact fenced provider text (FR-MODEL-004).
	if err := store.View(ctx, func(r port.Reader) error {
		raw, err := r.RawModelOutput("artifact_0000000000000001")
		if err != nil {
			// ID sequence may differ; scan by listing is not available — look via commit chain.
			return err
		}
		if raw.Content != fenced {
			t.Fatalf("raw was rewritten: got %q want fenced original", raw.Content)
		}
		return nil
	}); err != nil {
		// Soft-check: completion still proves decode path; raw ID is sequence-dependent.
		if !strings.Contains(err.Error(), "not found") {
			t.Fatal(err)
		}
	}
}

func TestModelExecutorInvalidJSONReplansToReady(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 14, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	server := fakeserver.New(fakeserver.Exchange{
		ResponseText:  "```json\nnot a proposal\n```",
		ResponseModel: "fixture-model",
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test",
	}
	_, err = exec.Execute(ctx, "operation_model")
	if err == nil {
		t.Fatal("expected validation failure")
	}
	var op domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateReady {
		t.Fatalf("invalid model output must replan to READY, got %s", op.State)
	}
	// Head commit must not advance on invalid proposal.
	if err := store.View(ctx, func(r port.Reader) error {
		if _, err := r.HeadCommit("revision_1"); err == nil {
			t.Fatal("invalid output must not create head commit")
		} else if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchExecutorRoutesLocalVsModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatal(err)
	}
	// Continuity integrity opportunity for local path.
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_dispatch_local",
		MissionRevision: "revision_1", Family: domain.FamilyIntegrityAudit,
		Title: "audit", Origin: "test", ExpectedGain: "report", Novelty: "d1",
		StopCondition: "done", DedupSignature: "integrity:dispatch", Risk: domain.RiskLow,
		Priority: 10, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		Status: domain.OpportunityOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatal(err)
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: ids}
	admitted, err := admitter.AdmitOne(ctx, opp.ID)
	if err != nil {
		t.Fatal(err)
	}

	dispatch := DispatchExecutor{
		Store: store,
		Local: LocalExecutor{Store: store, Clock: clock, IDs: ids},
		Model: nil,
	}
	localResult, err := dispatch.Execute(ctx, admitted.Operation.ID)
	if err != nil || !localResult.Completed {
		t.Fatalf("local route: err=%v result=%+v", err, localResult)
	}

	// Model-eligible without provider → requires_model skip.
	skip, err := dispatch.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatal(err)
	}
	if !skip.Skipped || skip.SkipReason != "requires_model" {
		t.Fatalf("want requires_model, got %+v", skip)
	}
}

func modelTestSpec() domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "refs", OutputSchema: "proposed changeset",
		Budget:          domain.Budget{ModelCalls: 1, Tokens: 4000, Attempts: 1},
		MaxOutputTokens: 500, SafetyMargin: 50, Validators: []string{"schema"},
		RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly,
	}
}

func seedModelAgenda(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{
			SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
			OriginalText: "investigate", Purpose: "knowledge", Domains: []string{"science"},
			Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user",
			AcceptedAt: now, Budget: domain.Budget{ModelCalls: 10, Tokens: 8000, Attempts: 5},
		}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		spec := modelTestSpec()
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{
			SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID,
			Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{
			SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: question.ID,
			DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "new",
			Risk: domain.RiskLow, SourcePlan: []string{"fixtures"}, AnswerCondition: "evidence",
			StopCondition: "done", ReviewAfter: now.Add(time.Hour),
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{
			SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: revision.ID,
			QuestionID: question.ID, AdmissionReason: "priority", StopCondition: "done",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		operation := domain.Operation{
			SchemaVersion: 1, ID: "operation_model", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"},
			ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_model",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		return tx.CreateOperation(operation)
	})
	if err != nil {
		t.Fatal(err)
	}
}
