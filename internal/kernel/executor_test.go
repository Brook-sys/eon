package kernel

import (
	"context"
	"errors"
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
			// Residual family depth fields are always present on local audits.
			if !strings.Contains(a.Content, `"family":"integrity_audit"`) {
				t.Fatalf("artifact missing family: %s", a.Content)
			}
			if !strings.Contains(a.Content, `"depth_max"`) {
				t.Fatalf("artifact missing depth_max: %s", a.Content)
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

func TestLocalAuditResidualFamilyDepth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}
	// Open opportunities at multiple depths/families feed residual histograms.
	// Active signature uniqueness is store-enforced, so depth is exercised via parent chain.
	opps := []domain.WorkOpportunity{
		{
			SchemaVersion: domain.SchemaVersionV1, ID: "opp_cov_1",
			MissionRevision: "revision_1", Family: domain.FamilyCoverageScan,
			Title: "coverage", Origin: "test", ExpectedGain: "cover", Novelty: "c1",
			StopCondition: "done", DedupSignature: "cov:1", Risk: domain.RiskLow, Priority: 10,
			EstimatedCost: domain.Budget{Tokens: 32, Attempts: 1}, Status: domain.OpportunityOpen,
			CreatedAt: now, UpdatedAt: now, Depth: 0,
		},
		{
			SchemaVersion: domain.SchemaVersionV1, ID: "opp_front_root",
			MissionRevision: "revision_1", Family: domain.FamilyFrontierManage,
			Title: "frontier root", Origin: "test", ExpectedGain: "frontier", Novelty: "f0",
			StopCondition: "done", DedupSignature: "front:root", Risk: domain.RiskLow, Priority: 10,
			EstimatedCost: domain.Budget{Tokens: 32, Attempts: 1}, Status: domain.OpportunityOpen,
			CreatedAt: now, UpdatedAt: now, Depth: 0,
		},
		{
			SchemaVersion: domain.SchemaVersionV1, ID: "opp_front_child",
			MissionRevision: "revision_1", Family: domain.FamilyFrontierManage,
			Title: "frontier child", Origin: "test", ExpectedGain: "frontier", Novelty: "f1",
			StopCondition: "done", DedupSignature: "front:child", Risk: domain.RiskLow, Priority: 10,
			EstimatedCost: domain.Budget{Tokens: 32, Attempts: 1}, Status: domain.OpportunityOpen,
			CreatedAt: now, UpdatedAt: now, ParentID: "opp_front_root", Depth: 1,
		},
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		for _, opp := range opps {
			if err := tx.CreateWorkOpportunity(opp); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Admit coverage scan → local continuity op.
	admitter := Admitter{Store: store, Clock: clock, IDs: ids, Catalog: DefaultFamilySpecCatalog()}
	admitted, err := admitter.AdmitOne(ctx, "opp_cov_1")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}
	result, err := exec.Execute(ctx, admitted.Operation.ID)
	if err != nil || !result.Completed {
		t.Fatalf("execute: err=%v result=%+v", err, result)
	}
	var content string
	if err := store.View(ctx, func(r port.Reader) error {
		arts, err := r.KnowledgeArtifacts()
		if err != nil {
			return err
		}
		for _, a := range arts {
			if a.ID == result.ArtifactID {
				content = a.Content
				if a.Kind != "coverage_scan_report" {
					t.Fatalf("kind = %q", a.Kind)
				}
				return nil
			}
		}
		return errors.New("artifact not found")
	}); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`"family":"mission_coverage_scan"`,
		`"depth_max":1`,
		`"frontier_duplicate_signature_count":0`,
		`"open_by_family"`,
		`"depth_histogram"`,
		`"admitted_by_family"`,
		`coverage:`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("artifact missing %q in %s", needle, content)
		}
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
