package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

type exitAfterCompletionProvider struct{ provider port.ModelProvider }

func (p exitAfterCompletionProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	result, err := p.provider.Complete(ctx, request)
	if err != nil {
		return result, err
	}
	// Terminate after the controlled HTTP exchange but before ModelExecutor can
	// append the completion receipt. Deferred cleanup must not run here.
	os.Exit(77)
	return port.CompletionResult{}, errors.New("unreachable after process exit")
}

type rejectRetransmissionProvider struct{ calls int }

func (p *rejectRetransmissionProvider) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	p.calls++
	return port.CompletionResult{}, errors.New("provider retransmission after ambiguous crash")
}

// TestModelExecutorProcessCrashAfterResponseBeforeReceipt is an integration
// boundary test rather than a core kernel test because it deliberately spawns
// and kills a subprocess. Core tests remain offline and process-free.
func TestModelExecutorProcessCrashAfterResponseBeforeReceipt(t *testing.T) {
	const helperEnv = "MOTOR_AUTONOMO_CRASH_AFTER_RESPONSE_HELPER"
	if os.Getenv(helperEnv) == "1" {
		runCrashAfterResponseHelper(t)
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "process-crash.sqlite")
	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_process_crash", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes:       []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_process_crash", PayloadRef: "payload_process_crash"}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(fakeserver.Exchange{ResponseText: string(body), ResponseModel: "fixture-model", InputTokens: 10, OutputTokens: 20})
	defer server.Close()

	cmd := exec.Command(os.Args[0], "-test.run", "^TestModelExecutorProcessCrashAfterResponseBeforeReceipt$")
	cmd.Env = append(os.Environ(), helperEnv+"=1", "MOTOR_AUTONOMO_CRASH_DB="+dbPath, "MOTOR_AUTONOMO_CRASH_SERVER="+server.URL())
	output, runErr := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 77 {
		t.Fatalf("crash helper exit=%v output=%s", runErr, output)
	}
	if got := len(server.Requests()); got != 1 {
		t.Fatalf("controlled provider requests=%d, want exactly one before crash", got)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.View(ctx, func(r port.Reader) error {
		reservations, err := r.ModelCallReservations("operation_model")
		if err != nil {
			return err
		}
		if len(reservations) != 1 || reservations[0].Attempt != 1 || reservations[0].ModelCall != 1 {
			return fmt.Errorf("reservations after crash = %#v", reservations)
		}
		if _, err := r.ModelCompletionReceipt("operation_model", 1, 1); !errors.Is(err, port.ErrNotFound) {
			return fmt.Errorf("completion receipt after crash err=%v, want not found", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	clock := source.NewManualClock(time.Date(2026, 7, 23, 5, 2, 0, 0, time.UTC))
	ids := source.NewSequenceIDGenerator(100)
	reconciled, err := (kernel.LeaseReaper{Store: store, Clock: clock, IDs: ids}).Reconcile(ctx, "revision_1")
	if err != nil || reconciled.Reconciled != 1 {
		t.Fatalf("reconcile=%+v err=%v", reconciled, err)
	}
	provider := &rejectRetransmissionProvider{}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@process-crash"})
	if err != nil {
		t.Fatal(err)
	}
	executor := kernel.ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@process-crash", LeaseTTL: time.Minute,
	}
	result, executeErr := executor.Execute(ctx, "operation_model")
	if executeErr == nil || !result.Exhausted || result.ModelCalls != 0 {
		t.Fatalf("restart result=%+v err=%v", result, executeErr)
	}
	if provider.calls != 0 || len(server.Requests()) != 1 {
		t.Fatalf("provider retransmission: local=%d controlled_total=%d", provider.calls, len(server.Requests()))
	}
	if err := store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_model")
		if err != nil {
			return err
		}
		if op.State != domain.StateExhausted || op.Attempt != 2 {
			return fmt.Errorf("operation after restart = state %s attempt %d", op.State, op.Attempt)
		}
		reservations, err := r.ModelCallReservations(op.ID)
		if err != nil {
			return err
		}
		if len(reservations) != 1 {
			return fmt.Errorf("reservation count after restart = %d", len(reservations))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func runCrashAfterResponseHelper(t *testing.T) {
	dbPath := os.Getenv("MOTOR_AUTONOMO_CRASH_DB")
	baseURL := os.Getenv("MOTOR_AUTONOMO_CRASH_SERVER")
	if dbPath == "" || baseURL == "" {
		t.Fatal("crash helper requires database and server")
	}
	now := time.Date(2026, 7, 23, 5, 0, 0, 0, time.UTC)
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	spec := processCrashModelSpec()
	seedProcessCrashAgenda(t, store, now, spec)
	provider, err := openai.New(openai.Config{BaseURL: baseURL, Model: "fixture-model"})
	if err != nil {
		t.Fatal(err)
	}
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@process-crash"})
	if err != nil {
		t.Fatal(err)
	}
	executor := kernel.ModelExecutor{
		Store: store, Clock: clock, IDs: ids,
		Provider: exitAfterCompletionProvider{provider: provider}, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@process-crash", LeaseTTL: time.Minute,
	}
	_, _ = executor.Execute(context.Background(), "operation_model")
	t.Fatal("crash helper returned without exiting")
}

func processCrashModelSpec() domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "refs", OutputSchema: "proposed changeset",
		Budget:          domain.Budget{ModelCalls: 1, Tokens: 4000, Attempts: 2},
		MaxOutputTokens: 500, SafetyMargin: 50, Validators: []string{"schema"},
		RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly,
	}
}

func seedProcessCrashAgenda(t *testing.T, store port.Store, now time.Time, spec domain.OperationSpec) {
	t.Helper()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "investigate", Purpose: "knowledge", Domains: []string{"science"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user", AcceptedAt: now, Budget: domain.Budget{ModelCalls: 10, Tokens: 8000, Attempts: 5}}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID, Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence"}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixtures"}, AnswerCondition: "evidence", StopCondition: "done", ReviewAfter: now.Add(time.Hour)}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "priority", StopCondition: "done", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		operation := domain.Operation{SchemaVersion: 1, ID: "operation_model", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_model", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		return tx.CreateOperation(operation)
	})
	if err != nil {
		t.Fatal(err)
	}
}
