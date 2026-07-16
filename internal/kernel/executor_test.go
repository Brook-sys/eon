package kernel

import (
	"context"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestLocalEligible(t *testing.T) {
	t.Parallel()
	readOnly := ContinuityOperationSpec("continuity.integrity_audit@1", domain.AuthorityReadOnly)
	if !LocalEligible(readOnly) {
		t.Fatal("read-only continuity must be local")
	}
	propose := ContinuityOperationSpec("continuity.gap_scan@1", domain.AuthorityProposeOnly)
	if !LocalEligible(propose) {
		t.Fatal("continuity.* propose-only is still model-free for the local executor")
	}
	extract := domain.OperationSpec{
		SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "text", OutputSchema: "json", Budget: domain.Budget{Tokens: 100},
		MaxOutputTokens: 20, SafetyMargin: 10, Validators: []string{"schema"},
		RetryPolicy: "none", FallbackPolicy: "none", MaximumAuthority: domain.AuthorityProposeOnly,
	}
	if LocalEligible(extract) {
		t.Fatal("non-continuity propose-only must wait for model path")
	}
}

func TestLocalExecutorCompletesContinuityOperation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	// Admit one integrity opportunity → READY continuity op.
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_integrity_1",
		MissionRevision: "revision_1", Family: domain.FamilyIntegrityAudit,
		Title: "audit store integrity", Origin: "test", ExpectedGain: "audit report",
		Novelty: "first audit", StopCondition: "report persisted",
		DedupSignature: "integrity:test", Risk: domain.RiskLow, Priority: 20,
		EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		Status:        domain.OpportunityOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatalf("seed opp: %v", err)
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: ids, Catalog: DefaultFamilySpecCatalog()}
	admitted, err := admitter.AdmitOne(ctx, opp.ID)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if admitted.Operation.State != domain.StateReady {
		t.Fatalf("want READY op, got %s", admitted.Operation.State)
	}

	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}
	result, err := exec.Execute(ctx, admitted.Operation.ID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Completed || result.Skipped || result.ArtifactID == "" || result.LeaseRef == "" {
		t.Fatalf("unexpected result: %+v", result)
	}

	var op domain.Operation
	var events []domain.Event
	var artifacts []domain.KnowledgeArtifact
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation(admitted.Operation.ID)
		if err != nil {
			return err
		}
		events, err = r.Events(0, 100)
		if err != nil {
			return err
		}
		artifacts, err = r.KnowledgeArtifacts()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateSucceeded || op.Attempt != 1 {
		t.Fatalf("operation after execute: %+v", op)
	}
	kinds := map[string]int{}
	for _, event := range events {
		if event.OperationID == admitted.Operation.ID {
			kinds[event.Kind]++
		}
	}
	for _, want := range []string{EventOperationDispatched, EventOperationLocalVerified, EventOperationSucceeded} {
		if kinds[want] != 1 {
			t.Fatalf("event %s count = %d, kinds=%v", want, kinds[want], kinds)
		}
	}
	if len(artifacts) == 0 {
		t.Fatal("expected audit artifact")
	}
	found := false
	for _, a := range artifacts {
		if a.ID == result.ArtifactID {
			found = true
			if a.Kind != "integrity_audit_report" {
				t.Fatalf("artifact kind = %q", a.Kind)
			}
			if !strings.Contains(a.Content, `"mode":"model_free_local"`) {
				t.Fatalf("artifact content missing mode: %s", a.Content)
			}
		}
	}
	if !found {
		t.Fatalf("artifact %s not stored", result.ArtifactID)
	}

	// Second execute is a no-op skip (terminal).
	again, err := exec.Execute(ctx, admitted.Operation.ID)
	if err != nil {
		t.Fatalf("re-execute: %v", err)
	}
	if !again.Skipped || again.SkipReason != "terminal" {
		t.Fatalf("expected terminal skip, got %+v", again)
	}
}

func TestLocalExecutorSkipsNonLocalSpec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedAgenda(t, store, now)

	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}
	result, err := exec.Execute(ctx, "operation_a")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Skipped || result.SkipReason != "requires_model" || result.Completed {
		t.Fatalf("want requires_model skip, got %+v", result)
	}
	var op domain.Operation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation("operation_a")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateReady {
		t.Fatalf("non-local op must remain READY, got %s", op.State)
	}
}

func TestProcessCyclePathViaSchedulerDispatchAndExecute(t *testing.T) {
	// Integration-style: scheduler DISPATCH then LocalExecutor completes.
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 12, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_gap_1",
		MissionRevision: "revision_1", Family: domain.FamilyGapScan,
		Title: "scan gaps", Origin: "test", ExpectedGain: "gaps",
		Novelty: "gap1", StopCondition: "listed", DedupSignature: "gap:test",
		Risk: domain.RiskLow, Priority: 40, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
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
		t.Fatalf("admit: %v", err)
	}

	scheduler := Scheduler{Store: store, Clock: clock, IDs: ids}
	decision, err := scheduler.Step(ctx, "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionDispatch || decision.Operation != admitted.Operation.ID {
		t.Fatalf("decision = %+v, want DISPATCH %s", decision, admitted.Operation.ID)
	}

	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}
	result, err := exec.Execute(ctx, decision.Operation)
	if err != nil || !result.Completed {
		t.Fatalf("execute after dispatch: err=%v result=%+v", err, result)
	}
}
