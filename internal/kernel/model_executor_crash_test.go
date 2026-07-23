package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/provider/openai/fakeserver"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

type ErrorProcessorTx struct {
	port.Transaction
}

func (e ErrorProcessorTx) CreateCommit(c domain.Commit) error {
	return errors.New("forced processor error")
}

func (e ErrorProcessorTx) AppendEvent(ev domain.Event) (domain.Event, error) {
	return e.Transaction.AppendEvent(ev)
}

func (e ErrorProcessorTx) CreateOperation(op domain.Operation) error {
	return e.Transaction.CreateOperation(op)
}

func (e ErrorProcessorTx) SaveOperation(op domain.Operation) error {
	return e.Transaction.SaveOperation(op)
}

func (e ErrorProcessorTx) AppendModelCompletionReceipt(r domain.ModelCompletionReceipt) error {
	return e.Transaction.AppendModelCompletionReceipt(r)
}

type ErrorProcessorStore struct {
	port.Store
}

func (e ErrorProcessorStore) Update(ctx context.Context, fn func(tx port.Transaction) error) error {
	return e.Store.Update(ctx, func(tx port.Transaction) error {
		return fn(ErrorProcessorTx{Transaction: tx})
	})
}

// TestModelExecutorCrashReplaySQLite proves that after a durable SUCCEED the
// model path is pure on reopen: re-Execute is terminal skip, commit/entity
// counts stay 1, and no second provider call is required for safety of state
// (provider is still constructed but skipped before Complete).
func TestModelExecutorCrashReplaySQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "model.sqlite")
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	ctx := context.Background()

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_crash", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_model_crash", PayloadRef: "payload_crash",
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
		InputTokens:   10,
		OutputTokens:  20,
	})
	defer server.Close()

	// Phase 1: open, seed, execute to completion, close.
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seedModelAgenda(t, store, now)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{
		Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@model-crash",
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
		PolicyVersion: "policy@model-crash",
		LeaseTTL:      5 * time.Minute,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed || result.CommitID == "" {
		t.Fatalf("result = %+v", result)
	}
	commitID := result.CommitID
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Phase 3: reopen and crash before completion receipt.
	proposal2 := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_crash2", MissionRevision: "revision_1",
		OperationID: "operation_model_2", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_model_crash2", PayloadRef: "payload_crash2",
		}},
		ExpectedDelta: "two observations", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model_2",
	}
	body2, err := json.Marshal(proposal2)
	if err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Re-add an operation linked to the already seeded mission/inquiry
	if err := store.Update(ctx, func(tx port.Transaction) error {
		op := domain.Operation{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "operation_model_2", InquiryID: "inquiry_1", MissionRevision: "revision_1", SpecID: "extract@1",
			State: domain.StateReady, ExpectedOutput: "test2", IdempotencyKey: "idem_model_2",
			Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
			InputRefs:    []string{"fragment_1"},
			ReadSet:      []string{"fragment_1"},
		}
		return tx.CreateOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	server3 := fakeserver.New(fakeserver.Exchange{
		ResponseText:  string(body2),
		ResponseModel: "fixture-model",
		InputTokens:   10,
		OutputTokens:  20,
		StatusCode:    500,
	})
	defer server3.Close()
	provider3, err := openai.New(openai.Config{BaseURL: server3.URL(), Model: "fixture-model", Client: server3.Client()})
	if err != nil {
		t.Fatal(err)
	}
	clock3 := source.NewManualClock(now.Add(2 * time.Minute))
	ids3 := source.NewSequenceIDGenerator(1000)
	processor3, err := changeset.New(changeset.Config{
		Store: store, Clock: clock3, IDs: ids3, PolicyVersion: "policy@model-crash",
	})
	if err != nil {
		t.Fatal(err)
	}
	exec3 := ModelExecutor{
		Store: store, Clock: clock3, IDs: ids3, Provider: provider3, Changes: processor3,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8000,
		},
		PolicyVersion: "policy@model-crash",
	}
	again3, _ := exec3.Execute(ctx, "operation_model_2")
	if again3.Completed || again3.Skipped {
		t.Fatalf("want no completion for 500, got %+v", again3)
	}
	_ = store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_model_2")
		if err == nil && op.State == domain.StateSucceeded {
			t.Fatal("operation state must not be SUCCEEDED if completion receipt fails")
		}
		return nil
	})
	_ = store.Close()
	// Phase 4: Reopen, execute to receipt boundary, crash before commit.
	// We achieve this by letting Complete succeed, but the processor returns an error.
	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Reset state of operation_model_2 to READY for phase 4
	_ = store.Update(ctx, func(tx port.Transaction) error {
		op, _ := tx.Operation("operation_model_2")
		op.State = domain.StateReady
		op.Attempt = 1 // Reset to attempt 1
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateReady}
		return tx.SaveOperation(op)
	})
	server4 := fakeserver.New(fakeserver.Exchange{
		ResponseText:  "invalid-json-causes-processor-error",
		ResponseModel: "fixture-model",
		InputTokens:   10,
		OutputTokens:  20,
	})
	defer server4.Close()
	provider4, err := openai.New(openai.Config{BaseURL: server4.URL(), Model: "fixture-model", Client: server4.Client()})
	if err != nil {
		t.Fatal(err)
	}
	clock4 := source.NewManualClock(now.Add(4 * time.Minute))
	ids4 := source.NewSequenceIDGenerator(2000)

	processor4, err := changeset.New(changeset.Config{
		Store: ErrorProcessorStore{Store: store},
		Clock: clock4, IDs: ids4, PolicyVersion: "policy@model-crash",
	})
	if err != nil {
		t.Fatal(err)
	}
	exec4 := ModelExecutor{
		Store: ErrorProcessorStore{Store: store}, Clock: clock4, IDs: ids4, Provider: provider4, Changes: processor4,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8000,
		},
		PolicyVersion: "policy@model-crash",
	}
	// Debug state before phase 4 execution
	_ = store.View(ctx, func(r port.Reader) error {
		op, _ := r.Operation("operation_model_2")
		t.Logf("State before Phase 4 Exec: %s", op.State)
		return nil
	})

	again4, err4 := exec4.Execute(ctx, "operation_model_2")
	t.Logf("again4 = %+v, err4 = %v", again4, err4)

	if again4.Completed || again4.Skipped {
		t.Fatalf("want no completion for invalid JSON, got %+v", again4)
	}
	// Verify receipt was stored despite the failure.
	_ = store.View(ctx, func(r port.Reader) error {
		op, err := r.Operation("operation_model_2")
		if err == nil && op.State == domain.StateSucceeded {
			t.Fatal("operation state must not be SUCCEEDED if processor fails")
		}

		// Load receipt
		_, err = r.ModelCompletionReceipt("operation_model_2", 2, 1)
		if err != nil {
			t.Fatalf("expected receipt to be persisted after provider call, got: %v", err)
		}
		return nil
	})

	// Execute Phase 4 again - should not hit provider, use receipt, and fail processing again.
	_, _ = exec4.Execute(ctx, "operation_model_2")
	if len(server4.Requests()) != 1 {
		t.Fatalf("expected 1 call to provider, got %d", len(server4.Requests()))
	}

	// Phase 2: reopen with fresh clocks/IDs/provider script and prove pure terminal skip.
	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Second exchange would be a bug if Complete is invoked after SUCCEED.
	server2 := fakeserver.New(fakeserver.Exchange{
		ResponseText:  `{"should":"not be called"}`,
		ResponseModel: "fixture-model",
	})
	defer server2.Close()
	provider2, err := openai.New(openai.Config{BaseURL: server2.URL(), Model: "fixture-model", Client: server2.Client()})
	if err != nil {
		t.Fatal(err)
	}
	clock2 := source.NewManualClock(now.Add(time.Minute))
	ids2 := source.NewSequenceIDGenerator(500)
	processor2, err := changeset.New(changeset.Config{
		Store: store, Clock: clock2, IDs: ids2, PolicyVersion: "policy@model-crash",
	})
	if err != nil {
		t.Fatal(err)
	}
	exec2 := ModelExecutor{
		Store: store, Clock: clock2, IDs: ids2, Provider: provider2, Changes: processor2,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8000,
		},
		PolicyVersion: "policy@model-crash",
	}
	again, err := exec2.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("re-execute after reopen: %v", err)
	}
	if !again.Skipped || again.SkipReason != "terminal" {
		t.Fatalf("want terminal skip after reopen, got %+v", again)
	}
	if len(server2.Requests()) != 0 {
		t.Fatalf("provider must not be called after durable SUCCEED, got %d calls", len(server2.Requests()))
	}

	var op domain.Operation
	var entity domain.CanonicalEntity
	var events []domain.Event
	var commits int
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		if err != nil {
			return err
		}
		entity, err = r.CanonicalEntity("observation", "obs_model_crash")
		if err != nil {
			return err
		}
		events, err = r.Events(0, 200)
		if err != nil {
			return err
		}
		// Count commits by walking BaseCommitID lineage from head.
		if head, headErr := r.HeadCommit("revision_1"); headErr == nil {
			for c := head; c.ID != domain.GenesisCommitID && c.ID != ""; {
				commits++
				if c.ID == commitID && c.BaseCommitID == domain.GenesisCommitID {
					break
				}
				parent, pErr := r.Commit(c.BaseCommitID)
				if pErr != nil {
					break
				}
				if parent.ID == c.ID {
					break
				}
				c = parent
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateSucceeded || op.Attempt != 1 {
		t.Fatalf("operation after reopen = %+v", op)
	}
	if entity.PayloadRef != "payload_crash" || entity.CommitID != commitID {
		t.Fatalf("entity = %+v want commit %s", entity, commitID)
	}
	if commits != 1 {
		// Single apply: one non-genesis commit on the mission head chain.
		t.Fatalf("expected exactly one commit on head chain, got %d", commits)
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
}

// TestModelExecutorReopenWhileRunningLeavesLeaseRecoverable seeds a RUNNING
// model op with an absolute lease deadline and proves LeaseReaper recovers it
// after SQLite reopen without inventing a second SUCCEED.
func TestModelExecutorReopenWhileRunningLeavesLeaseRecoverable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "model-running.sqlite")
	start := time.Date(2026, 7, 16, 15, 30, 0, 0, time.UTC)
	ctx := context.Background()

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seedModelAgenda(t, store, start)
	leaseRef := FormatLeaseRef("lease_mid", "operation_model", 1, start.Add(time.Minute))
	if err := store.Update(ctx, func(tx port.Transaction) error {
		op, err := tx.Operation("operation_model")
		if err != nil {
			return err
		}
		op.State = domain.StateRunning
		op.Attempt = 1
		op.Reevaluation = domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: leaseRef}
		return tx.SaveOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Advance past lease deadline and reconcile.
	clock := source.NewManualClock(start.Add(2 * time.Minute))
	ids := source.NewSequenceIDGenerator(10)
	reaper := LeaseReaper{Store: store, Clock: clock, IDs: ids}
	rec, err := reaper.Reconcile(ctx, "revision_1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.Reconciled != 1 {
		t.Fatalf("reconciled = %#v", rec)
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
		t.Fatalf("after expired lease reopen want READY, got %s", op.State)
	}
}
