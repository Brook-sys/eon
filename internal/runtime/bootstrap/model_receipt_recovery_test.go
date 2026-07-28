package bootstrap_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/mission"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/bootstrap"
)

func TestProcessCycleReconcilesModelCompletionReceiptAcrossSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	opts := modelRecoveryOptions(path, 8)

	first, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	op, spec := seedModelRecoveryOperation(t, first, "recovery-op")
	grant, err := first.Model.Authorizer.ReserveModelComplete(ctx, op, spec, 0, "", "")
	if err != nil || grant.Permit == nil {
		t.Fatalf("reserve: grant=%+v err=%v", grant, err)
	}
	makeRecoveryOperationTerminal(t, first.Store, op)
	appendRecoveryReceipt(t, first.Store, op.ID, 1, *grant.Permit, 17, 5)
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}

	second, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(ctx) })
	result, err := second.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelCompletionReceiptsReconciled != 1 {
		t.Fatalf("reconciled=%d result=%+v", result.ModelCompletionReceiptsReconciled, result)
	}
	if !result.SchedulerRan {
		t.Fatalf("scheduler did not run after successful reconciliation: %+v", result)
	}
	if err := second.Store.View(ctx, func(r port.Reader) error {
		receipt, err := r.ModelCompletionReceipt(op.ID, 1, 1)
		if err != nil {
			return err
		}
		if receipt.SettledAt == nil {
			t.Fatal("receipt remains unsettled")
		}
		usage, err := r.ResourceUsage(grant.Permit.Resource)
		if err != nil {
			return err
		}
		if usage.InFlight != 0 || usage.TokenMinuteCount != 22 {
			t.Fatalf("usage after recovery=%+v, want released and 22 observed tokens", usage)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessCycleModelCompletionReceiptBatchLeavesRemainder(t *testing.T) {
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, modelRecoveryOptions(filepath.Join(t.TempDir(), "runtime.sqlite"), 1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	op, spec := seedModelRecoveryOperation(t, rt, "batch-op")
	for call := uint32(1); call <= 2; call++ {
		grant, err := rt.Model.Authorizer.ReserveModelComplete(ctx, op, spec, 0, "", "")
		if err != nil || grant.Permit == nil {
			t.Fatalf("reserve %d: %+v %v", call, grant, err)
		}
		appendRecoveryReceipt(t, rt.Store, op.ID, call, *grant.Permit, int(call), 0)
	}
	makeRecoveryOperationTerminal(t, rt.Store, op)
	first, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.ModelCompletionReceiptsReconciled != 1 {
		t.Fatalf("first=%+v", first)
	}
	if err := rt.Store.View(ctx, func(r port.Reader) error {
		pending, err := r.UnsettledModelCompletionReceipts(8)
		if err == nil && len(pending) != 1 {
			t.Fatalf("pending=%d want=1", len(pending))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.ModelCompletionReceiptsReconciled != 1 {
		t.Fatalf("second=%+v", second)
	}
}

func TestProcessCycleFailsClosedOnUnsettleableModelReceipt(t *testing.T) {
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, modelRecoveryOptions(filepath.Join(t.TempDir(), "runtime.sqlite"), 8))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	seedModelRecoveryOperation(t, rt, "existing-op") // makes scheduler construction non-trivial
	result := domain.ModelCompletionResult{Text: "durable", InputTokens: 1, FinishReason: "stop"}
	hash, _ := result.Hash()
	receipt := domain.ModelCompletionReceipt{SchemaVersion: 1, OperationID: "missing-op", Attempt: 1, ModelCall: 1, Result: result, PayloadHash: hash, RecordedAt: time.Now().UTC(), Permits: []domain.ResourcePermit{{Resource: kernel.DefaultModelCompleteResource, Cost: domain.ResourceCost{Slots: 1, Calls: 1, Tokens: 1}, GrantedAt: time.Now().UTC()}}}
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error { return tx.AppendModelCompletionReceipt(receipt) }); err != nil {
		t.Fatal(err)
	}
	cycle, err := rt.ProcessCycle(ctx)
	if err == nil || !strings.Contains(err.Error(), "model completion receipt reconciliation") {
		t.Fatalf("cycle=%+v err=%v", cycle, err)
	}
	if cycle.SchedulerRan || cycle.SchedulerSteps != 0 {
		t.Fatalf("scheduler dispatched despite reconciliation error: %+v", cycle)
	}
}

func TestOptionsModelCompletionReceiptBatchValidation(t *testing.T) {
	opts := bootstrap.Options{}
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	if opts.ModelCompletionReceiptBatch != 8 {
		t.Fatalf("default=%d want=8", opts.ModelCompletionReceiptBatch)
	}
	if err := (&bootstrap.Options{ModelCompletionReceiptBatch: 257}).Validate(); err == nil {
		t.Fatal("batch >256 accepted")
	}
}

func modelRecoveryOptions(path string, batch int) bootstrap.Options {
	return bootstrap.Options{StoreBackend: bootstrap.StorageSQLite, SQLitePath: path, MissionID: "receipt-recovery", ModelCompletionReceiptBatch: batch, Model: &bootstrap.ModelOptions{Enabled: true, BaseURL: "http://127.0.0.1:1", Model: "unused"}}
}

func seedModelRecoveryOperation(t *testing.T, rt *bootstrap.Runtime, id domain.OperationID) (domain.Operation, domain.OperationSpec) {
	t.Helper()
	now := time.Now().UTC()
	loader := mission.Loader{Store: rt.Store, Clock: rt.Clock, IDs: rt.IDs}
	rev, err := loader.Load(context.Background(), []byte(`{"schema_version":1,"id":"receipt-recovery","revision":1,"original_text":"recover","purpose":"recover receipts","domains":["test"],"policies":["p"],"budget":{"model_calls":10,"tokens":10000,"bytes":1000,"attempts":10,"duration":60000000000},"status":"ACTIVE"}`), "test")
	if err != nil {
		t.Fatal(err)
	}
	spec := domain.OperationSpec{SchemaVersion: 1, ID: domain.OperationSpecID("spec-" + string(id)), ContractVersion: 1, TemplateVersion: 1, InputSchema: "refs", OutputSchema: "out", Budget: domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1}, MaxOutputTokens: 50, SafetyMargin: 1, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
	q := domain.Question{SchemaVersion: 1, ID: domain.QuestionID("q-" + string(id)), MissionRevision: rev.ID, Text: "q?", Origin: "test", Relevance: "primary", AnswerCondition: "done"}
	cand := domain.InquiryCandidate{SchemaVersion: 1, ID: domain.InquiryCandidateID("c-" + string(id)), MissionRevision: rev.ID, QuestionID: q.ID, DerivedFrom: []string{"test"}, ExpectedProgress: "progress", Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "done", StopCondition: "done", ReviewAfter: now.Add(time.Hour)}
	inq := domain.Inquiry{SchemaVersion: 1, ID: domain.InquiryID("i-" + string(id)), CandidateID: cand.ID, MissionRevision: rev.ID, QuestionID: q.ID, AdmissionReason: "test", StopCondition: "done", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	op := domain.Operation{SchemaVersion: 1, ID: id, InquiryID: inq.ID, MissionRevision: rev.ID, SpecID: spec.ID, ReadSet: []string{"input"}, ExpectedOutput: "out", IdempotencyKey: domain.IdempotencyKey("idem-" + string(id)), State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	if err := rt.Store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		if err := tx.CreateQuestion(q); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(cand); err != nil {
			return err
		}
		if err := tx.CreateInquiry(inq); err != nil {
			return err
		}
		return tx.CreateOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	return op, spec
}

func makeRecoveryOperationTerminal(t *testing.T, store port.Store, op domain.Operation) {
	t.Helper()
	op.State = domain.StateSucceeded
	op.Reevaluation = domain.ReevaluationCondition{}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveOperation(op) }); err != nil {
		t.Fatal(err)
	}
}

func appendRecoveryReceipt(t *testing.T, store port.Store, op domain.OperationID, call uint32, permit domain.ResourcePermit, input, output int) {
	t.Helper()
	result := domain.ModelCompletionResult{Text: "ok", InputTokens: input, OutputTokens: output, Model: "m", FinishReason: "stop"}
	hash, err := result.Hash()
	if err != nil {
		t.Fatal(err)
	}
	r := domain.ModelCompletionReceipt{SchemaVersion: 1, OperationID: op, Attempt: 1, ModelCall: call, Result: result, PayloadHash: hash, RecordedAt: time.Now().UTC(), Permits: []domain.ResourcePermit{permit}}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendModelCompletionReceipt(r) }); err != nil {
		t.Fatal(err)
	}
}
