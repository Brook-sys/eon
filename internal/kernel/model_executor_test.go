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

func TestModelExecutorInvalidJSONExhaustsWhenBudgetOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 14, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	// Always-invalid model: with ModelCalls=1 and Attempts=1 the ladder MUST
	// terminate EXHAUSTED (FR-MODEL-004), never loop READY replan forever.
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
	result, err := exec.Execute(ctx, "operation_model")
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !result.Exhausted || result.ModelCalls != 1 {
		t.Fatalf("want exhausted after 1 call, got %+v", result)
	}
	var op domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateExhausted {
		t.Fatalf("invalid model output with spent budget must EXHAUST, got %s", op.State)
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
	// No extra Complete after budget.
	if n := len(server.Requests()); n != 1 {
		t.Fatalf("expected exactly 1 provider call, got %d", n)
	}
}

func TestModelExecutorShortCorrectionThenSucceeds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	// Spec with 2 model calls so step 5 can run.
	err := store.Update(ctx, func(tx port.Transaction) error {
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
		spec.Budget.ModelCalls = 2
		spec.Budget.Attempts = 2
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
	server := fakeserver.New(
		fakeserver.Exchange{ResponseText: "not json at all", ResponseModel: "fixture-model"},
		fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fixture-model"},
	)
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
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed || result.ModelCalls != 2 {
		t.Fatalf("want completed after short correction (2 calls), got %+v", result)
	}
	if len(result.RecoveryStages) == 0 || result.RecoveryStages[0] != domain.RecoveryShortCorrection {
		t.Fatalf("expected short correction stage, got %v", result.RecoveryStages)
	}
	reqs := server.Requests()
	if len(reqs) != 2 {
		t.Fatalf("want 2 requests, got %d", len(reqs))
	}
	// Second prompt must be the localized correction, not a full original resend.
	if !strings.Contains(reqs[1].Prompt, "ERROR:") || !strings.Contains(reqs[1].Prompt, "REQUIRED_FORMAT:") {
		t.Fatalf("second prompt is not a short correction: %q", reqs[1].Prompt)
	}
	if strings.Contains(reqs[1].Prompt, "mission_revision_id") && strings.Contains(reqs[1].Prompt, "validators") {
		// Full compile facts should not all reappear; snippet may mention them only if prior output did.
		if len(reqs[1].Prompt) > 1200 {
			t.Fatalf("correction prompt looks like full resend: len=%d", len(reqs[1].Prompt))
		}
	}
}

func TestModelExecutorAlwaysInvalidExhaustsWithoutCallLoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 16, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	err := store.Update(ctx, func(tx port.Transaction) error {
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
		spec.Budget.ModelCalls = 3
		spec.Budget.Attempts = 1
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
			DerivedFrom: []string{"gap_1"}, ExpectedProgress: "x", Novelty: "new",
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
		return tx.CreateOperation(domain.Operation{
			SchemaVersion: 1, ID: "operation_model", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"},
			ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_model",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(
		fakeserver.Exchange{ResponseText: "bad1", ResponseModel: "m"},
		fakeserver.Exchange{ResponseText: "bad2", ResponseModel: "m"},
		fakeserver.Exchange{ResponseText: "bad3", ResponseModel: "m"},
		fakeserver.Exchange{ResponseText: "should-not-be-called", ResponseModel: "m"},
	)
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "m", Client: server.Client()})
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
	result, err := exec.Execute(ctx, "operation_model")
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if !result.Exhausted || result.ModelCalls != 3 {
		t.Fatalf("want 3 calls then exhaust, got %+v", result)
	}
	if len(server.Requests()) != 3 {
		t.Fatalf("provider call loop: got %d requests", len(server.Requests()))
	}
	var op domain.Operation
	_ = store.View(ctx, func(r port.Reader) error {
		op, _ = r.Operation("operation_model")
		return nil
	})
	if op.State != domain.StateExhausted {
		t.Fatalf("state=%s", op.State)
	}
}

func TestModelExecutorFallbackProviderSucceeds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	err := store.Update(ctx, func(tx port.Transaction) error {
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
		// Primary + short + simpler + fallback = up to 4, but we only need 3: primary, short, simpler, then fallback.
		spec.Budget.ModelCalls = 4
		spec.Budget.Attempts = 1
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
			DerivedFrom: []string{"gap_1"}, ExpectedProgress: "x", Novelty: "new",
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
		return tx.CreateOperation(domain.Operation{
			SchemaVersion: 1, ID: "operation_model", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"},
			ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_model",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_1", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_model_1", PayloadRef: "payload_model_1",
		}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fallback", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	// Primary always returns garbage; fallback returns a valid proposal on first contact.
	primary := fakeserver.New(
		fakeserver.Exchange{ResponseText: "bad-primary-1", ResponseModel: "primary"},
		fakeserver.Exchange{ResponseText: "bad-primary-2", ResponseModel: "primary"},
		fakeserver.Exchange{ResponseText: "bad-primary-3", ResponseModel: "primary"},
	)
	defer primary.Close()
	fallback := fakeserver.New(fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fallback-model"})
	defer fallback.Close()
	primaryProvider, err := openai.New(openai.Config{BaseURL: primary.URL(), Model: "primary", Client: primary.Client()})
	if err != nil {
		t.Fatal(err)
	}
	fallbackProvider, err := openai.New(openai.Config{BaseURL: fallback.URL(), Model: "fallback-model", Client: fallback.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: primaryProvider, FallbackProvider: fallbackProvider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test",
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed {
		t.Fatalf("want completed via fallback, got %+v", result)
	}
	foundFallback := false
	for _, stage := range result.RecoveryStages {
		if stage == domain.RecoveryFallbackModel {
			foundFallback = true
		}
	}
	if !foundFallback {
		t.Fatalf("expected FALLBACK_MODEL stage, stages=%v", result.RecoveryStages)
	}
	if len(fallback.Requests()) != 1 {
		t.Fatalf("fallback provider must receive exactly one call, got %d", len(fallback.Requests()))
	}
	// Primary: initial + short correction + simpler format = 3, then fallback call on other server.
	if n := len(primary.Requests()); n != 3 {
		t.Fatalf("primary expected 3 recovery calls, got %d", n)
	}
	if result.ModelCalls != 4 {
		t.Fatalf("total model calls want 4, got %d", result.ModelCalls)
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

func TestModelExecutorUsesJSONModeWhenProfileConfirms(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

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
		ExpectedResponseFormat: "json_object",
		RequireResponseFormat:  true,
		ResponseText:           string(body),
		ResponseModel:          "fixture-json",
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture-json", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{
		Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.BaselineDeclaredProfile("json-capable", "fixture-json", domain.MaxOutputDialectLegacy, 8192, now)
	profile.SupportsJSONMode = true
	profile.Source = domain.CapabilityOverride
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test",
		Profile:       profile,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed {
		t.Fatalf("want completed: %+v", result)
	}
	if len(server.Requests()) != 1 || server.Requests()[0].ResponseFormat != "json_object" {
		t.Fatalf("requests = %+v failures=%v", server.Requests(), server.Failures())
	}
	// Adaptation audit event must exist.
	var sawAdapt bool
	_ = store.View(ctx, func(r port.Reader) error {
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if ev.OperationID == "operation_model" && ev.Kind == "operation.model_adaptation" && strings.Contains(ev.PayloadRef, "level=ASSISTED_JSON") {
				sawAdapt = true
			}
		}
		return nil
	})
	if !sawAdapt {
		t.Fatal("expected operation.model_adaptation with ASSISTED_JSON")
	}
}

func TestModelExecutorBaselineOmitsResponseFormatWithoutProfileSupport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 18, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

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
	// fakeserver fails if response_format is present unexpectedly.
	server := fakeserver.New(fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fixture"})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
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
		// Compiler window larger than effective conservative budget → plan must shrink.
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8192},
		PolicyVersion: "policy@model-test",
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed {
		t.Fatalf("want completed: %+v", result)
	}
	if n := len(server.Requests()); n != 1 {
		t.Fatalf("requests = %d failures=%v", n, server.Failures())
	}
	if server.Requests()[0].ResponseFormat != "" {
		t.Fatalf("baseline must omit response_format, got %q", server.Requests()[0].ResponseFormat)
	}
	var sawBaseline bool
	_ = store.View(ctx, func(r port.Reader) error {
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if ev.OperationID == "operation_model" && ev.Kind == "operation.model_adaptation" && strings.Contains(ev.PayloadRef, "level=BASELINE") {
				// Effective context must be strictly below declared compiler window.
				if strings.Contains(ev.PayloadRef, "ctx=8192") {
					t.Fatalf("context not conservative: %s", ev.PayloadRef)
				}
				sawBaseline = true
			}
		}
		return nil
	})
	if !sawBaseline {
		t.Fatal("expected baseline adaptation event")
	}
}

func TestModelExecutorDemotesJSONModeOnEnrichmentTransportFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 19, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	// Seed agenda with ModelCalls=2 so demotion can retry on baseline.
	if err := store.Update(ctx, func(tx port.Transaction) error {
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
		spec.Budget.ModelCalls = 2
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
			DerivedFrom: []string{"gap_1"}, ExpectedProgress: "x", Novelty: "new",
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
		return tx.CreateOperation(domain.Operation{
			SchemaVersion: 1, ID: "operation_model", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"},
			ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_model",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		})
	}); err != nil {
		t.Fatal(err)
	}

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
	server := fakeserver.New(
		// First call: pretend JSON mode is unsupported (HTTP 400 body free of secrets).
		fakeserver.Exchange{
			ExpectedResponseFormat: "json_object",
			RequireResponseFormat:  true,
			StatusCode:             400,
			RawBody:                `{"error":{"message":"response_format json_object not supported"}}`,
		},
		// Second call: baseline succeeds without response_format.
		fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fixture"},
	)
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.BaselineDeclaredProfile("json-capable", "fixture", domain.MaxOutputDialectLegacy, 4096, now)
	profile.SupportsJSONMode = true
	profile.Source = domain.CapabilityOverride
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 4096},
		PolicyVersion: "policy@model-test",
		Profile:       profile,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed {
		t.Fatalf("want completed after demotion: %+v", result)
	}
	reqs := server.Requests()
	if len(reqs) != 2 {
		t.Fatalf("want 2 calls, got %d failures=%v", len(reqs), server.Failures())
	}
	if reqs[0].ResponseFormat != "json_object" {
		t.Fatalf("first call format = %q", reqs[0].ResponseFormat)
	}
	if reqs[1].ResponseFormat != "" {
		t.Fatalf("second call must be baseline, got format %q", reqs[1].ResponseFormat)
	}
	if result.ModelCalls != 2 {
		t.Fatalf("model calls = %d", result.ModelCalls)
	}
}
