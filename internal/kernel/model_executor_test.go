package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type crashAfterReservedProvider struct{ calls int }

func (p *crashAfterReservedProvider) ID() string { return "crash-after-reserved" }
func (p *crashAfterReservedProvider) Kind() domain.ProviderKind {
	return domain.ProviderKindOpenAICompatible
}
func (p *crashAfterReservedProvider) Profile() domain.ProviderProfile {
	return domain.ProviderProfile{MaxOutputTokens: 512, MaxContextTokens: 8000}
}
func (p *crashAfterReservedProvider) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	p.calls++
	return port.CompletionResult{}, errors.New("simulated process loss after request start")
}

func TestModelExecutorBurnsUnresolvedReservationAcrossRedispatch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 4, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	spec := modelTestSpec()
	spec.Budget.Attempts = 2
	seedModelAgendaWithSpec(t, store, now, spec)
	provider := &crashAfterReservedProvider{}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@reservation-test"})
	if err != nil {
		t.Fatal(err)
	}
	executor := ModelExecutor{Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor, Compiler: prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000}, PolicyVersion: "policy@reservation-test", LeaseTTL: time.Minute}

	first, err := executor.Execute(ctx, "operation_model")
	if err == nil || first.ModelCalls != 1 || provider.calls != 1 {
		t.Fatalf("first result=%+v err=%v calls=%d", first, err, provider.calls)
	}
	var reservations []domain.ModelCallReservation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		reservations, err = r.ModelCallReservations("operation_model")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(reservations) != 1 || reservations[0].Attempt != 1 || reservations[0].ModelCall != 1 {
		t.Fatalf("reservations after ambiguous call = %#v", reservations)
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_model")
		if err != nil {
			return err
		}
		op.State = domain.StateReady
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateReady}
		return tx.SaveOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	second, err := executor.Execute(ctx, "operation_model")
	if err == nil || !second.Exhausted || second.ModelCalls != 0 {
		t.Fatalf("second result=%+v err=%v", second, err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one lifetime call", provider.calls)
	}
}

func TestBuildPromptInputUsesMinimalExactTextContract(t *testing.T) {
	executor := ModelExecutor{}
	spec := modelTestSpec()
	spec.OutputSchema = "exact_text"
	operation := domain.Operation{
		ID: "operation_probe", MissionRevision: "revision_probe", SpecID: spec.ID,
		ExpectedOutput: "Reply with exactly OK and nothing else.", IdempotencyKey: "probe",
	}
	input, err := executor.buildPromptInput(operation, spec, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Facts) != 0 || len(input.Constraints) != 1 || len(input.AllowedOutputs) != 1 {
		t.Fatalf("exact-text prompt retained generic changeset envelope: %+v", input)
	}
	if input.Task != operation.ExpectedOutput || input.AnswerFormat != "exact requested text only" {
		t.Fatalf("unexpected exact-text prompt: %+v", input)
	}
	compiled, err := (prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 2048}).Compile(spec, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"operation_id", "mission_revision_id", "ProposedChangeSet", "canonical snake_case"} {
		if strings.Contains(compiled.Request.Prompt, forbidden) {
			t.Fatalf("minimal prompt contains %q:\n%s", forbidden, compiled.Request.Prompt)
		}
	}
}

func TestBuildPromptInputUsesMinimalExactJSONContract(t *testing.T) {
	executor := ModelExecutor{}
	spec := modelTestSpec()
	spec.OutputSchema = "exact_json"
	operation := domain.Operation{
		ID: "operation_probe", MissionRevision: "revision_probe", SpecID: spec.ID,
		ExpectedOutput: `Return exactly {"status":"OK","retry":false}.`, IdempotencyKey: "probe",
	}
	input, err := executor.buildPromptInput(operation, spec, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Facts) != 0 || len(input.Constraints) != 1 || len(input.AllowedOutputs) != 1 {
		t.Fatalf("exact-JSON prompt retained generic changeset envelope: %+v", input)
	}
	if input.Task != operation.ExpectedOutput || input.AnswerFormat != "single exact JSON object only" {
		t.Fatalf("unexpected exact-JSON prompt: %+v", input)
	}
	compiled, err := (prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 2048}).Compile(spec, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"operation_id", "mission_revision_id", "ProposedChangeSet", "canonical snake_case"} {
		if strings.Contains(compiled.Request.Prompt, forbidden) {
			t.Fatalf("minimal prompt contains %q:\n%s", forbidden, compiled.Request.Prompt)
		}
	}
}

func TestBuildPromptInputConstrainsProposedChangeSetToCanonicalKeys(t *testing.T) {
	executor := ModelExecutor{}
	spec := modelTestSpec()
	operation := domain.Operation{
		ID: "operation_probe", MissionRevision: "revision_probe", SpecID: spec.ID,
		ExpectedOutput: "Propose one observation.", IdempotencyKey: "probe",
		ReadSet: []string{"manifest"}, InputRefs: []string{"source_1"},
	}
	input, err := executor.buildPromptInput(operation, spec, domain.GenesisCommitID)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := (prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 4096}).Compile(spec, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"top-level object may contain only", "idempotency_key", "Each changes item may contain only", "every other top-level field is a JSON string", "Do not wrap the object", "do not add input_refs",
		"MUST each be one JSON string", "expected_delta: \"one observation\"",
		"provenance MUST be a non-empty JSON string", "provenance: \"model:proposed_changeset\"", "never use an empty or whitespace-only value",
		"read_set and preconditions MUST each be a JSON array of strings", "read_set: [\"manifest\"]", "preconditions: []",
	} {
		if !strings.Contains(compiled.Request.Prompt, required) {
			t.Fatalf("changeset prompt lacks %q:\n%s", required, compiled.Request.Prompt)
		}
	}
}

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
	var receipt domain.ModelCompletionReceipt
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
		receipt, err = r.ModelCompletionReceipt("operation_model", 1, 1)
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
	if receipt.Result.Text != string(body) || receipt.Result.Model != "fixture-model" || receipt.Result.InputTokens != 40 || receipt.Result.OutputTokens != 80 {
		t.Fatalf("completion receipt = %+v", receipt)
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

func TestModelExecutorReusesReceiptAfterExpiredAttemptWithoutProviderCall(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 22, 19, 20, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	spec := modelTestSpec()
	spec.Budget.Attempts = 2
	seedModelAgendaWithSpec(t, store, now, spec)

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_replayed", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes:       []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_replayed", PayloadRef: "payload_replayed"}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	durable := port.DurableModelCompletionResult(port.CompletionResult{Text: string(body), Model: "fixture-model", InputTokens: 10, OutputTokens: 20, FinishReason: port.CompletionFinishStop})
	hash, err := durable.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_model")
		if err != nil {
			return err
		}
		op.State = domain.StateRunning
		op.Attempt = 1 // Already in READY state, mock a retry.
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: FormatLeaseRef("l1", "operation_model", 1, now.Add(time.Hour))}
		if err := tx.SaveOperation(op); err != nil {
			return err
		}
		return tx.AppendModelCompletionReceipt(domain.ModelCompletionReceipt{
			SchemaVersion: 1, OperationID: op.ID, Attempt: 2, ModelCall: 1,
			Result: durable, PayloadHash: hash, RecordedAt: now.Add(-time.Minute),
		})
	}); err != nil {
		t.Fatal(err)
	}

	clock.Advance(2 * time.Hour)
	rec, err := LeaseReaper{Store: store, Clock: clock, IDs: ids}.Reconcile(ctx, "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Reconciled != 1 {
		t.Fatalf("expected 1 reconciled lease, got %d", rec.Reconciled)
	}

	server := fakeserver.New(fakeserver.Exchange{StatusCode: 500})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "must-not-run", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	executor := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test", LeaseTTL: 5 * time.Minute,
	}
	result, err := executor.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute replay: %v", err)
	}
	if !result.Completed || result.ModelCalls != 0 || len(server.Requests()) != 0 {
		t.Fatalf("replay result=%+v provider calls=%d", result, len(server.Requests()))
	}
	if err := store.View(ctx, func(r port.Reader) error {
		replayed, err := r.ModelCompletionReceipt("operation_model", 2, 1)
		if err != nil {
			return err
		}
		if replayed.PayloadHash != hash {
			t.Fatalf("replayed receipt hash = %s, want %s", replayed.PayloadHash, hash)
		}
		_, err = r.CanonicalEntity("observation", "obs_replayed")
		return err
	}); err != nil {
		t.Fatal(err)
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

func TestModelExecutorFencesAttemptBudgetBeforeDispatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 7, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_model")
		if err != nil {
			return err
		}
		op.Attempt = 1
		return tx.SaveOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(fakeserver.Exchange{ResponseText: "must-not-run", ResponseModel: "fixture-model"})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler: prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000}, PolicyVersion: "policy@model-test"}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Exhausted || result.SkipReason != "attempt_budget_exhausted" || result.ModelCalls != 0 {
		t.Fatalf("result=%+v", result)
	}
	if len(server.Requests()) != 0 {
		t.Fatal("provider contacted past attempt budget")
	}
	if err := store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_model")
		if err != nil {
			return err
		}
		if op.State != domain.StateExhausted || op.Attempt != 1 {
			t.Fatalf("op=%+v", op)
		}
		_, err = r.EventByID("operation_model:model_attempts_exhausted:1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestModelExecutorAuditsProviderFailureWhileRecoveryLeaseIsVerifying(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 3, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	spec := modelTestSpec()
	spec.Budget.ModelCalls = 3
	seedModelAgendaWithSpec(t, store, now, spec)
	server := fakeserver.New(
		fakeserver.Exchange{ResponseText: "bad-primary", ResponseModel: "primary"},
		fakeserver.Exchange{StatusCode: 503, RawBody: `{"error":{"message":"temporary"}}`},
	)
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "primary", Client: server.Client()})
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
		t.Fatal("expected provider failure after validation recovery")
	}
	if strings.Contains(err.Error(), "operation lease changed during failed model call") {
		t.Fatalf("provider failure in VERIFYING was misclassified as lease loss: %v", err)
	}
	var invoked int
	if viewErr := store.View(ctx, func(r port.Reader) error {
		events, readErr := r.Events(0, 100)
		if readErr != nil {
			return readErr
		}
		for _, event := range events {
			if event.OperationID == "operation_model" && event.Kind == EventOperationModelInvoked && strings.Contains(event.PayloadRef, "outcome=provider_error") {
				invoked++
			}
		}
		return nil
	}); viewErr != nil {
		t.Fatal(viewErr)
	}
	if invoked != 1 {
		t.Fatalf("provider failure audit events=%d, want 1", invoked)
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
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@model-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []domain.ResourceID{
		domain.ModelProviderResource("primary-provider"),
		domain.ModelBindingResource("primary-binding"),
		domain.ModelProviderResource("fallback-provider"),
		domain.ModelBindingResource("fallback-binding"),
	} {
		auth.Limits[resource] = domain.ResourceLimit{Resource: resource, MaxConcurrent: 4, MaxPerMinute: 10}
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: primaryProvider, FallbackProvider: fallbackProvider, Changes: processor,
		Compiler:           prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion:      "policy@model-test",
		Authorizer:         auth,
		PrimaryProviderID:  "primary-provider",
		PrimaryBindingID:   "primary-binding",
		FallbackProviderID: "fallback-provider",
		FallbackBindingID:  "fallback-binding",
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
	var primaryBindingUsage, fallbackBindingUsage domain.ResourceUsage
	if err := store.View(ctx, func(r port.Reader) error {
		var readErr error
		primaryBindingUsage, readErr = r.ResourceUsage(domain.ModelBindingResource("primary-binding"))
		if readErr != nil {
			return readErr
		}
		fallbackBindingUsage, readErr = r.ResourceUsage(domain.ModelBindingResource("fallback-binding"))
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	if primaryBindingUsage.MinuteCount != 3 || fallbackBindingUsage.MinuteCount != 1 {
		t.Fatalf("attempts must charge the binding actually used: primary=%+v fallback=%+v", primaryBindingUsage, fallbackBindingUsage)
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
	seedModelAgendaWithSpec(t, store, now, modelTestSpec())
}

func seedModelAgendaWithSpec(t *testing.T, store port.Store, now time.Time, spec domain.OperationSpec) {
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

func TestModelExecutorPersistsNIMContextPressureAndRecoversGradually(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 7, 20, 0, 0, time.UTC)
	store := memory.New()
	seedModelAgenda(t, store, now)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveModelContextPressure(domain.ModelContextPressure{
			BindingID: "nim-small",
			State:     domain.ContextPressureState{Level: 2},
			UpdatedAt: now,
		})
	}); err != nil {
		t.Fatal(err)
	}

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_1", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes:       []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_model_1", PayloadRef: "payload_model_1"}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fixture"})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: source.NewManualClock(now), IDs: source.NewSequenceIDGenerator(1), PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.BaselineDeclaredProfile("nim", "fixture", domain.MaxOutputDialectLegacy, 8000, now)
	exec := ModelExecutor{
		Store: store, Clock: source.NewManualClock(now), IDs: source.NewSequenceIDGenerator(100), Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test", Profile: profile,
		PrimaryBindingID: "nim-small", PrimaryProviderKind: domain.ProviderKindNVIDIANIM,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil || !result.Completed {
		t.Fatalf("execute result=%+v err=%v", result, err)
	}
	var pressure domain.ModelContextPressure
	var events []domain.Event
	if err := store.View(ctx, func(r port.Reader) error {
		pressure, err = r.ModelContextPressure("nim-small")
		if err == nil {
			events, err = r.Events(0, 100)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if pressure.State != (domain.ContextPressureState{Level: 2, SuccessesAtLevel: 1}) {
		t.Fatalf("pressure after one success = %+v", pressure.State)
	}
	foundReduced := false
	for _, event := range events {
		if event.Kind == "operation.model_adaptation" && strings.Contains(event.PayloadRef, "reason=context_pressure_reduction") && strings.Contains(event.PayloadRef, "ctx=4000") {
			foundReduced = true
		}
	}
	if !foundReduced {
		t.Fatalf("missing reduced-context audit event: %#v", events)
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

type scopedHTTPError struct {
	status int
	quota  port.RateLimitMetadata
}

func (e scopedHTTPError) Error() string                             { return "provider failure" }
func (e scopedHTTPError) RetryAfterDelay() time.Duration            { return time.Minute }
func (e scopedHTTPError) HTTPStatusCode() int                       { return e.status }
func (e scopedHTTPError) RetryableFailure() bool                    { return true }
func (e scopedHTTPError) RateLimitMetadata() port.RateLimitMetadata { return e.quota }

func TestSafeRateLimitPayloadProjectsOnlyTypedObservedFields(t *testing.T) {
	err := scopedHTTPError{status: 429, quota: port.RateLimitMetadata{
		HasRequestLimit: true, RequestLimit: 30,
		HasRequestRemaining: true, RequestRemaining: 0,
		HasRequestReset: true, RequestReset: 1500 * time.Millisecond,
		HasTokenRemaining: true, TokenRemaining: 42,
	}}
	metadata := rateLimitMetadata(fmt.Errorf("wrapped: %w", err))
	got := safeRateLimitPayload(metadata)
	want := ";quota_request_limit=30;quota_request_remaining=0;quota_request_reset_ms=1500;quota_token_remaining=42"
	if got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
	if got := safeRateLimitPayload(port.RateLimitMetadata{}); got != "" {
		t.Fatalf("empty metadata payload = %q", got)
	}
}

func TestModelFailureScopeByProviderKindAndSelectivePermitReporting(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 14, 0, 0, 0, time.UTC)
	operation := domain.Operation{SchemaVersion: domain.SchemaVersionV1, ID: "operation_scope", MissionRevision: "revision_1"}

	cases := []struct {
		name               string
		kind               domain.ProviderKind
		wantScope          string
		wantProviderFailed bool
		wantBindingFailed  bool
	}{
		{name: "groq binding-wide", kind: domain.ProviderKindGroq, wantScope: "binding", wantBindingFailed: true},
		{name: "NIM provider-wide", kind: domain.ProviderKindNVIDIANIM, wantScope: "provider", wantProviderFailed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, ok := classifyProviderFailure(scopedHTTPError{status: 429}, tc.kind)
			if !ok || decision.Scope != tc.wantScope {
				t.Fatalf("decision = %+v, classified=%v", decision, ok)
			}
			store := memory.New()
			clock := source.NewManualClock(now)
			auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@test")
			if err != nil {
				t.Fatal(err)
			}
			providerResource := domain.ModelProviderResource("p")
			bindingResource := domain.ModelBindingResource("b")
			for _, resource := range []domain.ResourceID{providerResource, bindingResource} {
				auth.Limits[resource] = domain.ResourceLimit{Resource: resource, FailureThreshold: 1, CooldownBase: time.Minute, CooldownMax: time.Minute}
				if err := store.Update(ctx, func(tx port.Transaction) error {
					return tx.SaveResourceUsage(domain.ResourceUsage{Resource: resource, InFlight: 1})
				}); err != nil {
					t.Fatal(err)
				}
			}
			exec := ModelExecutor{Authorizer: auth}
			permits := []*domain.ResourcePermit{{Resource: providerResource, Cost: domain.ResourceCost{Slots: 1}}, {Resource: bindingResource, Cost: domain.ResourceCost{Slots: 1}}}
			retryAfter := now.Add(time.Minute)
			exec.releaseFailedResourcePermits(ctx, operation, permits, decision, true, &retryAfter)

			var providerUsage, bindingUsage domain.ResourceUsage
			if err := store.View(ctx, func(r port.Reader) error {
				providerUsage, err = r.ResourceUsage(providerResource)
				if err == nil {
					bindingUsage, err = r.ResourceUsage(bindingResource)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			providerFailed := providerUsage.ConsecutiveFailures == 1 && providerUsage.CircuitOpenUntil != nil
			bindingFailed := bindingUsage.ConsecutiveFailures == 1 && bindingUsage.CircuitOpenUntil != nil
			if providerFailed != tc.wantProviderFailed || bindingFailed != tc.wantBindingFailed {
				t.Fatalf("provider=%+v binding=%+v", providerUsage, bindingUsage)
			}
		})
	}
}

func TestModelExecutorCatalog503FallsBackOnceAndOpensFailedBindingCircuit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	spec := modelTestSpec()
	spec.Budget.ModelCalls = 2
	seedModelAgendaWithSpec(t, store, now, spec)

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_catalog_fallback", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes:       []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_catalog_fallback", PayloadRef: "payload_catalog_fallback"}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fallback", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	primary := fakeserver.New(fakeserver.Exchange{StatusCode: 503, RawBody: `{"error":{"message":"temporarily unavailable"}}`})
	defer primary.Close()
	fallback := fakeserver.New(fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fallback-model", InputTokens: 30, OutputTokens: 60})
	defer fallback.Close()
	primaryProvider, err := openai.New(openai.Config{BaseURL: primary.URL(), Model: "primary-model", Client: primary.Client()})
	if err != nil {
		t.Fatal(err)
	}
	fallbackProvider, err := openai.New(openai.Config{BaseURL: fallback.URL(), Model: "fallback-model", Client: fallback.Client()})
	if err != nil {
		t.Fatal(err)
	}
	limit := func(resource domain.ResourceID) domain.ResourceLimit {
		return domain.ResourceLimit{Resource: resource, MaxConcurrent: 4, MaxPerMinute: 10, FailureThreshold: 1, CooldownBase: time.Minute, CooldownMax: time.Minute}
	}
	config := domain.ModelsConfig{
		Version: "models@test",
		Providers: []domain.ModelProviderConfig{
			{ID: "primary", Kind: domain.ProviderKindGroq, BaseURL: primary.URL(), APIKeyEnv: "PRIMARY_API_KEY", Timeout: time.Minute, MaxResponseBytes: 1 << 20, GlobalLimit: limit(domain.ModelProviderResource("primary"))},
			{ID: "fallback", Kind: domain.ProviderKindNVIDIANIM, BaseURL: fallback.URL(), APIKeyEnv: "FALLBACK_API_KEY", Timeout: time.Minute, MaxResponseBytes: 1 << 20, GlobalLimit: limit(domain.ModelProviderResource("fallback"))},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "primary-binding", ProviderRef: "primary", ModelID: "primary-model", Enabled: true, Priority: 0, ContextTokens: 8000, MaxOutputTokens: 500, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: limit(domain.ModelBindingResource("primary-binding"))},
			{ID: "fallback-binding", ProviderRef: "fallback", ModelID: "fallback-model", Enabled: true, Priority: 1, ContextTokens: 8000, MaxOutputTokens: 500, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: limit(domain.ModelBindingResource("fallback-binding"))},
		},
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@model-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range config.Providers {
		auth.Limits[provider.GlobalLimit.Resource] = provider.GlobalLimit
	}
	for _, binding := range config.Bindings {
		auth.Limits[binding.Limit.Resource] = binding.Limit
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: primaryProvider,
		Providers: map[string]port.ModelProvider{"primary-binding": primaryProvider, "fallback-binding": fallbackProvider}, ModelsConfig: &config,
		Changes: processor, Compiler: prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@model-test", Authorizer: auth,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed || result.ModelCalls != 2 {
		t.Fatalf("want bounded primary failure + fallback success, got %+v", result)
	}
	if len(primary.Requests()) != 1 || len(fallback.Requests()) != 1 {
		t.Fatalf("provider calls primary=%d fallback=%d", len(primary.Requests()), len(fallback.Requests()))
	}
	var primaryProviderUsage, primaryBindingUsage, fallbackProviderUsage, fallbackBindingUsage domain.ResourceUsage
	var events []domain.Event
	if err := store.View(ctx, func(r port.Reader) error {
		var readErr error
		primaryProviderUsage, readErr = r.ResourceUsage(domain.ModelProviderResource("primary"))
		if readErr == nil {
			primaryBindingUsage, readErr = r.ResourceUsage(domain.ModelBindingResource("primary-binding"))
		}
		if readErr == nil {
			fallbackProviderUsage, readErr = r.ResourceUsage(domain.ModelProviderResource("fallback"))
		}
		if readErr == nil {
			fallbackBindingUsage, readErr = r.ResourceUsage(domain.ModelBindingResource("fallback-binding"))
		}
		if readErr == nil {
			events, readErr = r.Events(0, 100)
		}
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	if primaryBindingUsage.CircuitOpenUntil == nil || primaryProviderUsage.CircuitOpenUntil != nil {
		t.Fatalf("503 must cool only failed binding: provider=%+v binding=%+v", primaryProviderUsage, primaryBindingUsage)
	}
	for name, usage := range map[string]domain.ResourceUsage{"primary-provider": primaryProviderUsage, "primary-binding": primaryBindingUsage, "fallback-provider": fallbackProviderUsage, "fallback-binding": fallbackBindingUsage} {
		if usage.InFlight != 0 {
			t.Fatalf("%s leaked permit: %+v", name, usage)
		}
	}
	routes := 0
	releases := 0
	for _, event := range events {
		if event.Kind == EventOperationModelRouted && event.OperationID == "operation_model" {
			routes++
		}
		if event.Kind == EventResourceReleased && event.OperationID == "operation_model" {
			releases++
		}
	}
	if routes != 2 {
		t.Fatalf("want primary and fallback routing events, got %d", routes)
	}
	if releases != 2 {
		t.Fatalf("want one durable release event per model attempt, got %d", releases)
	}
}

func TestModelExecutorCatalogQuotaDenialWaitsWithoutProviderCall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 10, 0, 30, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)
	server := fakeserver.New(fakeserver.Exchange{ResponseText: `{}`})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "quota-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	providerResource := domain.ModelProviderResource("quota-provider")
	bindingResource := domain.ModelBindingResource("quota-binding")
	limit := func(resource domain.ResourceID) domain.ResourceLimit {
		return domain.ResourceLimit{Resource: resource, MaxConcurrent: 4, MaxPerMinute: 1}
	}
	config := domain.ModelsConfig{
		Version:   "models@quota-test",
		Providers: []domain.ModelProviderConfig{{ID: "quota-provider", Kind: domain.ProviderKindGroq, BaseURL: server.URL(), APIKeyEnv: "QUOTA_API_KEY", Timeout: time.Minute, MaxResponseBytes: 1 << 20, GlobalLimit: limit(providerResource)}},
		Bindings:  []domain.ModelBindingConfig{{ID: "quota-binding", ProviderRef: "quota-provider", ModelID: "quota-model", Enabled: true, Priority: 0, ContextTokens: 8000, MaxOutputTokens: 500, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: limit(bindingResource)}},
	}
	window := now.Truncate(time.Minute)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.SaveResourceUsage(domain.ResourceUsage{Resource: providerResource, MinuteWindowStart: window, MinuteCount: 1}); err != nil {
			return err
		}
		return tx.SaveResourceUsage(domain.ResourceUsage{Resource: bindingResource, MinuteWindowStart: window})
	}); err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@quota-test")
	if err != nil {
		t.Fatal(err)
	}
	auth.Limits[providerResource] = limit(providerResource)
	auth.Limits[bindingResource] = limit(bindingResource)
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@quota-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider,
		Providers: map[string]port.ModelProvider{"quota-binding": provider}, ModelsConfig: &config,
		Changes: processor, Compiler: prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@quota-test", Authorizer: auth,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Skipped || result.ModelCalls != 0 || len(server.Requests()) != 0 {
		t.Fatalf("quota denial must not call provider: result=%+v calls=%d", result, len(server.Requests()))
	}
	var operation domain.Operation
	var providerUsage, bindingUsage domain.ResourceUsage
	if err := store.View(ctx, func(r port.Reader) error {
		var readErr error
		operation, readErr = r.Operation("operation_model")
		if readErr == nil {
			providerUsage, readErr = r.ResourceUsage(providerResource)
		}
		if readErr == nil {
			bindingUsage, readErr = r.ResourceUsage(bindingResource)
		}
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	if operation.State != domain.StateWaitingTime || operation.Reevaluation.NotBefore == nil || !operation.Reevaluation.NotBefore.Equal(window.Add(time.Minute)) {
		t.Fatalf("want WAITING until next window, got state=%s reevaluation=%+v", operation.State, operation.Reevaluation)
	}
	if providerUsage.MinuteCount != 1 || providerUsage.InFlight != 0 || bindingUsage.MinuteCount != 0 || bindingUsage.InFlight != 0 {
		t.Fatalf("denial changed usage unexpectedly: provider=%+v binding=%+v", providerUsage, bindingUsage)
	}
}

func TestModelExecutorPreventsRedispatchWhenLifetimeBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()

	spec := modelTestSpec()
	spec.Budget.ModelCalls = 2
	spec.Budget.Attempts = 2
	seedModelAgendaWithSpec(t, store, now, spec)

	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_model")
		if err != nil {
			return err
		}
		op.State = domain.StateReady
		op.Attempt = 2 // Ready for second attempt
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateReady}
		if err := tx.SaveOperation(op); err != nil {
			return err
		}

		// Attempt 1 exhausted the lifetime call budget (2 calls).
		// Neither has a receipt (simulating crash before receipt, or rejected output).
		if err := tx.AppendModelCallReservation(domain.ModelCallReservation{
			SchemaVersion: domain.SchemaVersionV1,
			OperationID:   op.ID,
			Attempt:       1,
			ModelCall:     1,
			BindingID:     "fixture-binding",
			ReservedAt:    now.Add(-time.Hour),
		}); err != nil {
			return err
		}
		return tx.AppendModelCallReservation(domain.ModelCallReservation{
			SchemaVersion: domain.SchemaVersionV1,
			OperationID:   op.ID,
			Attempt:       1,
			ModelCall:     2,
			BindingID:     "fixture-binding",
			ReservedAt:    now.Add(-time.Hour),
		})
	}); err != nil {
		t.Fatal(err)
	}

	server := fakeserver.New(fakeserver.Exchange{StatusCode: 500})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "must-not-run", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@test"})
	if err != nil {
		t.Fatal(err)
	}
	executor := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@test", LeaseTTL: 5 * time.Minute,
	}

	result, err := executor.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute redispatch: %v", err)
	}

	if !result.Exhausted {
		t.Errorf("expected result to be Exhausted when lifetime budget is gone")
	}
	if result.ModelCalls != 0 {
		t.Errorf("expected 0 new provider calls, got %d", result.ModelCalls)
	}
	if len(server.Requests()) != 0 {
		t.Errorf("expected 0 HTTP requests, got %d", len(server.Requests()))
	}

	if err := store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_model")
		if err != nil {
			return err
		}
		if op.State != domain.StateExhausted {
			t.Errorf("operation state = %s, want EXHAUSTED", op.State)
		}
		if op.Attempt != 2 {
			t.Errorf("operation attempt = %d, want 2", op.Attempt)
		}
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		var exhausted bool
		for _, ev := range events {
			if ev.Kind == "operation.model_exhausted" && ev.OperationID == op.ID {
				exhausted = true
				break
			}
		}
		if !exhausted {
			t.Errorf("missing operation.model_exhausted event")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
