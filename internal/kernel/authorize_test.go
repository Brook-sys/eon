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

func TestReserveModelCompleteAllowsAndPersistsUsage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 21, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedModelAgenda(t, store, now)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@res-test")
	if err != nil {
		t.Fatal(err)
	}
	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	out, err := auth.ReserveModelComplete(ctx, op, spec, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Allowed || out.Permit == nil {
		t.Fatalf("want allow+permit, got %+v", out)
	}
	if out.Permit.Resource != DefaultModelCompleteResource {
		t.Fatalf("resource = %s", out.Permit.Resource)
	}

	var usage domain.ResourceUsage
	var kinds map[string]int
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		usage, err = r.ResourceUsage(DefaultModelCompleteResource)
		if err != nil {
			return err
		}
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		kinds = map[string]int{}
		for _, e := range events {
			if e.OperationID == op.ID {
				kinds[e.Kind]++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if usage.InFlight != 1 || usage.MinuteCount != 1 {
		t.Fatalf("usage after reserve = %+v", usage)
	}
	if kinds[EventCapabilityAuthorized] != 1 {
		t.Fatalf("authorized events = %v", kinds)
	}

	if err := auth.ReportModelCompleteObserved(ctx, op, []*domain.ResourcePermit{out.Permit}, true, nil, 100); err != nil {
		t.Fatal(err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		usage, err = r.ResourceUsage(DefaultModelCompleteResource)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if usage.InFlight != 0 || usage.MinuteCount != 1 {
		t.Fatalf("usage after success report = %+v", usage)
	}
	// Token reconciliation should replace the conservative estimate with 100 observed tokens.
	if usage.TokenMinuteCount != 100 {
		t.Fatalf("usage tokens after success report = %d", usage.TokenMinuteCount)
	}
}

func TestReserveModelCompleteThrottlesWhenConcurrencySaturated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 21, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedModelAgenda(t, store, now)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@res-test")
	if err != nil {
		t.Fatal(err)
	}
	// Force single-slot concurrency so the second reserve throttles.
	auth.Limits[DefaultModelCompleteResource] = domain.ResourceLimit{
		Resource:      DefaultModelCompleteResource,
		MaxConcurrent: 1,
		MaxPerMinute:  30,
		MaxPerDay:     500,
	}

	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	first, err := auth.ReserveModelComplete(ctx, op, spec, 0, "", "")
	if err != nil || !first.Allowed {
		t.Fatalf("first reserve: %+v err=%v", first, err)
	}

	// Second concurrent operation: same mission but distinct id for throttle transition.
	op2 := op
	op2.ID = "operation_model_2"
	op2.IdempotencyKey = "idem_model_2"
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateOperation(op2)
	}); err != nil {
		t.Fatal(err)
	}

	second, err := auth.ReserveModelComplete(ctx, op2, spec, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Allowed || !second.Throttled {
		t.Fatalf("want throttled, got %+v", second)
	}
	if !strings.Contains(second.SkipReason, "resource_") {
		t.Fatalf("skip reason = %q", second.SkipReason)
	}

	var got domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		got, err = r.Operation(op2.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got.State != domain.StateThrottled && got.State != domain.StateWaitingTime {
		t.Fatalf("throttled op state = %s reeval=%+v", got.State, got.Reevaluation)
	}

	// Provider must not be called when executor is wired with authorizer.
	// Covered by TestModelExecutorAuthorizerThrottlesWithoutProviderCall.
}

func TestReserveModelCompleteDeniesWithoutPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 21, 20, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedModelAgenda(t, store, now)

	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@res-test")
	if err != nil {
		t.Fatal(err)
	}
	auth.GrantedPermissions = nil // fail closed

	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		if err != nil {
			return err
		}
		spec, err = r.OperationSpec(op.SpecID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	out, err := auth.ReserveModelComplete(ctx, op, spec, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Allowed || out.Throttled {
		t.Fatalf("want policy deny, got %+v", out)
	}
	if out.SkipReason != "policy_deny" {
		t.Fatalf("skip = %q", out.SkipReason)
	}

	var kinds map[string]int
	if err := store.View(ctx, func(r port.Reader) error {
		events, err := r.Events(0, 50)
		if err != nil {
			return err
		}
		kinds = map[string]int{}
		for _, e := range events {
			if e.OperationID == op.ID {
				kinds[e.Kind]++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if kinds[EventCapabilityDenied] != 1 {
		t.Fatalf("denied events = %v", kinds)
	}
	// No usage row without acquire.
	if err := store.View(ctx, func(r port.Reader) error {
		_, err := r.ResourceUsage(DefaultModelCompleteResource)
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("usage err = %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestModelExecutorWithAuthorizerCompletesAndReleases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 21, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_1", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_auth_1", PayloadRef: "payload_auth_1",
		}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
		Provenance: "model:fixture", IdempotencyKey: "idem_model",
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	server := fakeserver.New(fakeserver.Exchange{
		ResponseText: string(body), ResponseModel: "fixture-model", InputTokens: 10, OutputTokens: 20,
	})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture-model", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@res-test"})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@res-test")
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@res-test", Authorizer: auth,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed {
		t.Fatalf("result = %+v", result)
	}
	if len(server.Requests()) != 1 {
		t.Fatalf("provider calls = %d", len(server.Requests()))
	}

	var usage domain.ResourceUsage
	var kinds map[string]int
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		usage, err = r.ResourceUsage(DefaultModelCompleteResource)
		if err != nil {
			return err
		}
		events, err := r.Events(0, 100)
		if err != nil {
			return err
		}
		kinds = map[string]int{}
		for _, e := range events {
			if e.OperationID == "operation_model" {
				kinds[e.Kind]++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if usage.InFlight != 0 {
		t.Fatalf("in-flight must release after success: %+v", usage)
	}
	if kinds[EventCapabilityAuthorized] != 1 || kinds[EventResourceReleased] != 1 {
		t.Fatalf("auth events = %v", kinds)
	}
}

func TestModelExecutorAuthorizerThrottlesWithoutProviderCall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 21, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	// Pre-saturate the gate so Execute throttles before Complete.
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.SaveResourceUsage(domain.ResourceUsage{
			Resource: DefaultModelCompleteResource, InFlight: 2,
			MinuteWindowStart: now.Truncate(time.Minute), MinuteCount: 2,
		})
	}); err != nil {
		t.Fatal(err)
	}

	server := fakeserver.New(fakeserver.Exchange{ResponseText: "{}", ResponseModel: "should-not-run"})
	defer server.Close()
	provider, err := openai.New(openai.Config{BaseURL: server.URL(), Model: "fixture", Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@res-test"})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@res-test")
	if err != nil {
		t.Fatal(err)
	}
	// MaxConcurrent 2 matches seeded InFlight=2 → denial.
	auth.Limits[DefaultModelCompleteResource] = domain.ResourceLimit{
		Resource: DefaultModelCompleteResource, MaxConcurrent: 2, MaxPerMinute: 30, MaxPerDay: 500,
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@res-test", Authorizer: auth,
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || !strings.HasPrefix(result.SkipReason, "resource_") {
		t.Fatalf("want resource skip, got %+v", result)
	}
	if n := len(server.Requests()); n != 0 {
		t.Fatalf("provider must not be called when throttled, got %d", n)
	}
	var op domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_model")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateThrottled && op.State != domain.StateWaitingTime {
		t.Fatalf("operation state = %s", op.State)
	}
}

func TestModelExecutorAuthorizerDisabledKeepsLegacyPath(t *testing.T) {
	t.Parallel()
	// Nil authorizer must still complete (opt-in enforcement).
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 21, 50, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedModelAgenda(t, store, now)

	proposal := domain.ProposedChangeSet{
		SchemaVersion: 1, ID: "changeset_model_1", MissionRevision: "revision_1",
		OperationID: "operation_model", BaseCommitID: domain.GenesisCommitID,
		ReadSet: []string{"fragment_1"}, Preconditions: []string{},
		Changes: []domain.Change{{
			Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_legacy", PayloadRef: "payload_legacy",
		}},
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
	processor, err := changeset.New(changeset.Config{Store: store, Clock: clock, IDs: ids, PolicyVersion: "policy@res-test"})
	if err != nil {
		t.Fatal(err)
	}
	exec := ModelExecutor{
		Store: store, Clock: clock, IDs: ids, Provider: provider, Changes: processor,
		Compiler:      prompt.Compiler{Estimator: prompt.ConservativeEstimator{}, ProviderContextTokens: 8000},
		PolicyVersion: "policy@res-test",
		// Authorizer nil
	}
	result, err := exec.Execute(ctx, "operation_model")
	if err != nil || !result.Completed {
		t.Fatalf("legacy path: %+v err=%v", result, err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		_, err := r.ResourceUsage(DefaultModelCompleteResource)
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("nil authorizer must not write usage: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSettleModelCompletionReceiptReleasesReconcilesAndReplayIsNoOp(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 28, 17, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedModelAgenda(t, store, now)
	auth, err := NewMVPCapabilityAuthorizer(store, clock, "policy@test")
	if err != nil {
		t.Fatal(err)
	}
	var op domain.Operation
	var spec domain.OperationSpec
	if err := store.View(ctx, func(r port.Reader) error {
		var e error
		op, e = r.Operation("operation_model")
		if e == nil {
			spec, e = r.OperationSpec(op.SpecID)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	grant, err := auth.ReserveModelComplete(ctx, op, spec, 0, "", "")
	if err != nil || grant.Permit == nil {
		t.Fatalf("grant=%+v err=%v", grant, err)
	}
	result := domain.ModelCompletionResult{InputTokens: 40, OutputTokens: 60, Model: "m", FinishReason: "stop"}
	hash, _ := result.Hash()
	receipt := domain.ModelCompletionReceipt{SchemaVersion: 1, OperationID: op.ID, Attempt: 1, ModelCall: 1, Result: result, PayloadHash: hash, RecordedAt: now, Permits: []domain.ResourcePermit{*grant.Permit}}
	if err := store.Update(ctx, func(tx port.Transaction) error { return tx.AppendModelCompletionReceipt(receipt) }); err != nil {
		t.Fatal(err)
	}
	if err := auth.SettleModelCompletionReceipt(ctx, op, receipt); err != nil {
		t.Fatal(err)
	}
	if err := auth.SettleModelCompletionReceipt(ctx, op, receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.View(ctx, func(r port.Reader) error {
		u, e := r.ResourceUsage(grant.Permit.Resource)
		if e != nil {
			return e
		}
		if u.InFlight != 0 || u.TokenMinuteCount != 100 {
			t.Fatalf("usage=%+v", u)
		}
		got, e := r.ModelCompletionReceipt(op.ID, 1, 1)
		if e != nil {
			return e
		}
		if got.SettledAt == nil {
			t.Fatal("receipt not settled")
		}
		events, e := r.Events(0, 100)
		if e != nil {
			return e
		}
		releases := 0
		for _, event := range events {
			if event.Kind == EventResourceReleased {
				releases++
			}
		}
		if releases != 1 {
			t.Fatalf("release events=%d", releases)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestModelCompletionReceiptRejectsInvalidAndDuplicateDurablePermits(t *testing.T) {
	now := time.Date(2026, 7, 28, 18, 0, 0, 0, time.UTC)
	result := domain.ModelCompletionResult{Text: "x"}
	hash, _ := result.Hash()
	base := domain.ModelCompletionReceipt{SchemaVersion: 1, OperationID: "op", Attempt: 1, ModelCall: 1, Result: result, PayloadHash: hash, RecordedAt: now}
	for name, permits := range map[string][]domain.ResourcePermit{
		"invalid":   {{Resource: "r", Cost: domain.ResourceCost{Slots: -1}, GrantedAt: now}},
		"duplicate": {{Resource: "r", Cost: domain.ResourceCost{Slots: 1}, GrantedAt: now}, {Resource: "r", Cost: domain.ResourceCost{Slots: 1}, GrantedAt: now}},
	} {
		t.Run(name, func(t *testing.T) {
			receipt := base
			receipt.Permits = permits
			if err := receipt.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}
