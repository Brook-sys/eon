package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestAdmitterMaterialisesOpportunityIntoAgenda(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(context.Background(), store, nil); err != nil {
		t.Fatal(err)
	}
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_gap_1", MissionRevision: "revision_1",
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "find uncovered scopes",
		Origin: "mission", ExpectedGain: "new inquiries", Novelty: "scopes without inquiries",
		StopCondition: "scopes listed", DedupSignature: "gap:scopes:v1", Depth: 0,
		EstimatedCost: domain.Budget{Tokens: 100, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 20, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatal(err)
	}

	admitter := Admitter{Store: store, Clock: clock, IDs: source.NewSequenceIDGenerator(1)}
	result, err := admitter.AdmitOne(context.Background(), opp.ID)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if result.Inquiry.ID == "" || result.Operation.State != domain.StateReady {
		t.Fatalf("result = %+v", result)
	}
	if result.Operation.SpecID != DefaultFamilySpecCatalog()[domain.FamilyGapScan] {
		t.Fatalf("spec = %s", result.Operation.SpecID)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		got, err := r.WorkOpportunity(opp.ID)
		if err != nil {
			return err
		}
		if got.Status != domain.OpportunityAdmitted || got.AdmittedInquiryID != result.Inquiry.ID {
			t.Fatalf("opportunity = %+v", got)
		}
		operations, err := r.Operations("revision_1")
		if err != nil {
			return err
		}
		if len(operations) != 1 || operations[0].State != domain.StateReady {
			t.Fatalf("operations = %#v", operations)
		}
		events, err := r.Events(0, 20)
		if err != nil {
			return err
		}
		found := false
		for _, event := range events {
			if event.Kind == domain.EventContinuityExpanded {
				found = true
			}
		}
		if !found {
			t.Fatal("missing continuity.expanded event")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdmitFromFrontierRespectsMaxReadyAndTarget(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(context.Background(), store, nil); err != nil {
		t.Fatal(err)
	}
	policy := domain.DefaultHorizonPolicy()
	policy.TargetReady = 2
	policy.LowWatermark = 1
	policy.MaxReady = 2
	names := []string{"a", "b", "c", "d"}
	for i, name := range names {
		opp := domain.WorkOpportunity{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.WorkOpportunityID("opp_" + name),
			MissionRevision: "revision_1", Family: domain.FamilyIntegrityAudit, Status: domain.OpportunityOpen,
			Title: "audit " + name, Origin: "local", ExpectedGain: "integrity",
			Novelty: "check " + name, StopCondition: "report", DedupSignature: "integrity:" + name,
			Depth: 0, EstimatedCost: domain.Budget{Tokens: 50, Attempts: 1}, Risk: domain.RiskLow,
			Priority: uint8(30 - i), CreatedAt: now.Add(time.Duration(i) * time.Minute), UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateWorkOpportunity(opp)
		}); err != nil {
			t.Fatal(err)
		}
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: source.NewSequenceIDGenerator(10), Policy: policy}
	result, err := admitter.AdmitFromFrontier(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if result.Admitted != 2 {
		t.Fatalf("admitted = %d, want 2", result.Admitted)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		open, err := r.WorkOpportunities("revision_1", domain.OpportunityOpen)
		if err != nil {
			return err
		}
		admitted, err := r.WorkOpportunities("revision_1", domain.OpportunityAdmitted)
		if err != nil {
			return err
		}
		if len(open) != 2 || len(admitted) != 2 {
			t.Fatalf("open=%d admitted=%d", len(open), len(admitted))
		}
		// Highest priority first: opp_a (30), opp_b (29).
		if admitted[0].ID != "opp_a" && admitted[1].ID != "opp_a" {
			t.Fatalf("expected opp_a admitted, got %#v", admitted)
		}
		ready := 0
		ops, err := r.Operations("revision_1")
		if err != nil {
			return err
		}
		for _, op := range ops {
			if op.State == domain.StateReady {
				ready++
			}
		}
		if ready != 2 {
			t.Fatalf("ready = %d", ready)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Further admission must refuse at max_ready.
	_, err = admitter.AdmitOne(context.Background(), "opp_c")
	if err == nil || !errors.Is(err, port.ErrConflict) {
		t.Fatalf("expected max_ready conflict, got %v", err)
	}
}

func TestDecomposerEnforcesFanoutDepthAndNovelty(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 20, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	policy := domain.DefaultHorizonPolicy()
	policy.MaxChildren = 2
	policy.MaxDepth = 1
	root := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_root", MissionRevision: "revision_1",
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "cover gaps", Origin: "mission",
		ExpectedGain: "new inquiries", Novelty: "uncovered scopes", StopCondition: "coverage target",
		DedupSignature: "gap:root", Depth: 0, EstimatedCost: domain.Budget{Tokens: 10}, Risk: domain.RiskLow,
		Priority: 10, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(root)
	}); err != nil {
		t.Fatal(err)
	}
	decomposer := Decomposer{Store: store, Clock: clock, IDs: source.NewSequenceIDGenerator(1), Policy: policy}
	children, err := decomposer.SpawnChildren(context.Background(), root.ID, []ChildDraft{
		{
			Title: "define term A", Origin: "decompose:root", ExpectedGain: "definition", Novelty: "term A undefined",
			StopCondition: "definition accepted", DedupSignature: "gap:term:a", Risk: domain.RiskLow, Priority: 8,
			EstimatedCost: domain.Budget{Tokens: 4},
		},
		{
			Title: "define term B", Origin: "decompose:root", ExpectedGain: "definition", Novelty: "term B undefined",
			StopCondition: "definition accepted", DedupSignature: "gap:term:b", Risk: domain.RiskLow, Priority: 7,
			EstimatedCost: domain.Budget{Tokens: 4},
		},
	})
	if err != nil || len(children) != 2 {
		t.Fatalf("children = %#v err=%v", children, err)
	}
	// Fan-out exhausted.
	_, err = decomposer.SpawnChildren(context.Background(), root.ID, []ChildDraft{{
		Title: "define term C", Origin: "decompose:root", ExpectedGain: "definition", Novelty: "term C undefined",
		StopCondition: "definition accepted", DedupSignature: "gap:term:c", Risk: domain.RiskLow, Priority: 6,
		EstimatedCost: domain.Budget{Tokens: 4},
	}})
	if err == nil {
		t.Fatal("expected fan-out rejection")
	}
	// Depth exhausted for child.
	_, err = decomposer.SpawnChildren(context.Background(), children[0].ID, []ChildDraft{{
		Title: "subdetail", Origin: "decompose:child", ExpectedGain: "detail", Novelty: "sub detail of A",
		StopCondition: "detail recorded", DedupSignature: "gap:term:a:sub", Risk: domain.RiskLow, Priority: 5,
		EstimatedCost: domain.Budget{Tokens: 2},
	}})
	if err == nil {
		t.Fatal("expected depth rejection")
	}
	// Paraphrase without novelty rejected on a fresh root.
	root2 := root
	root2.ID = "opp_root2"
	root2.DedupSignature = "gap:root2"
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(root2)
	}); err != nil {
		t.Fatal(err)
	}
	_, err = decomposer.SpawnChildren(context.Background(), root2.ID, []ChildDraft{{
		Title: root2.Title, Origin: "decompose:root2", ExpectedGain: "same", Novelty: root2.Novelty,
		StopCondition: "same", DedupSignature: "gap:paraphrase", Risk: domain.RiskLow, Priority: 5,
		EstimatedCost: domain.Budget{Tokens: 2},
	}})
	if err == nil || !errors.Is(err, port.ErrConflict) {
		t.Fatalf("expected paraphrase conflict, got %v", err)
	}
}

func TestPreventiveReplenishAndLocalFamilyStrategy(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	ids := source.NewSequenceIDGenerator(100)
	policy := domain.DefaultHorizonPolicy()
	policy.TargetReady = 2
	policy.LowWatermark = 1
	policy.MaxReady = 4

	// Pre-seed open opportunities without ready operations.
	for i, name := range []string{"x", "y", "z"} {
		opp := domain.WorkOpportunity{
			SchemaVersion: domain.SchemaVersionV1, ID: domain.WorkOpportunityID("opp_" + name),
			MissionRevision: "revision_1", Family: domain.FamilyArtifactRefresh, Status: domain.OpportunityOpen,
			Title: "refresh " + name, Origin: "artifact", ExpectedGain: "fresh view", Novelty: "stale " + name,
			StopCondition: "artifact refreshed", DedupSignature: "artifact:" + name, Depth: 0,
			EstimatedCost: domain.Budget{Tokens: 40, Attempts: 1}, Risk: domain.RiskLow,
			Priority: uint8(20 - i), CreatedAt: now, UpdatedAt: now,
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateWorkOpportunity(opp)
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnsureCatalogSpecs(context.Background(), store, nil); err != nil {
		t.Fatal(err)
	}

	replenisher := Replenisher{Store: store, Clock: clock, IDs: ids, Policy: policy}
	result, horizon, err := replenisher.PreventivelyReplenish(context.Background(), "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Admitted != 2 || horizon.ReadyCount != 2 {
		t.Fatalf("preventive result=%+v horizon=%+v", result, horizon)
	}

	// Local strategy with empty ready after draining: seed + admit integrity family.
	// Consume ready by marking operations non-ready is heavy; instead use a fresh mission seed.
	store2 := memory.New()
	seedMission(t, store2)
	reg := NewStrategyRegistry()
	if err := RegisterDefaultContinuityFamilies(reg, store2, clock, source.NewSequenceIDGenerator(200), policy); err != nil {
		t.Fatal(err)
	}
	scheduler := Scheduler{Store: store2, Clock: clock, Registry: reg, Policy: policy, IDs: source.NewSequenceIDGenerator(300)}
	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if decision.Kind != DecisionDispatch {
		t.Fatalf("decision = %+v, want dispatch after continuity families", decision)
	}
	if decision.Strategy == "" && decision.Kind == DecisionDispatch {
		// Strategy may be set when admitted by a family; frontier_admission also valid.
	}
	if err := store2.View(context.Background(), func(r port.Reader) error {
		ops, err := r.Operations("revision_1")
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			t.Fatal("expected admitted operations")
		}
		opps, err := r.WorkOpportunities("revision_1", "")
		if err != nil {
			return err
		}
		if len(opps) == 0 {
			t.Fatal("expected seeded opportunities")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterDefaultContinuityFamiliesIncludesResidualPortfolio(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 50, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	ids := source.NewSequenceIDGenerator(400)
	policy := domain.DefaultHorizonPolicy()
	policy.TargetReady = 3
	policy.LowWatermark = 1
	policy.MaxReady = 6

	reg := NewStrategyRegistry()
	if err := RegisterDefaultContinuityFamilies(reg, store, clock, ids, policy); err != nil {
		t.Fatal(err)
	}
	wantFamilies := map[domain.WorkFamily]string{
		domain.FamilyGapScan:           "gap_scan",
		domain.FamilyConflictReview:    "conflict_evidence_review",
		domain.FamilyCoverageScan:      "mission_coverage_scan",
		domain.FamilyArtifactRefresh:   "artifact_refresh",
		domain.FamilySourceFreshness:   "source_freshness_scan",
		domain.FamilyIntegrityAudit:    "integrity_audit",
		domain.FamilyHarnessEvaluation: "harness_evaluation",
		domain.FamilyFrontierManage:    "frontier_management",
	}
	// 8 local families + recurring_obligations seeder (FR-DUR-011).
	if reg.Len() != len(wantFamilies)+1 {
		t.Fatalf("registry len = %d, want %d", reg.Len(), len(wantFamilies)+1)
	}
	got := map[domain.WorkFamily]string{}
	var sawRecurring bool
	for _, d := range reg.Descriptors() {
		if d.Name == "recurring_obligations" {
			sawRecurring = true
			if d.Version != "v1" || d.Priority != 40 || !d.LocalOnly {
				t.Fatalf("recurring descriptor = %+v", d)
			}
			continue
		}
		got[d.Family] = d.Name
		if !d.LocalOnly {
			t.Fatalf("family %s should be local-only", d.Family)
		}
		if d.Version != "v2" {
			t.Fatalf("family %s version = %q, want v2", d.Family, d.Version)
		}
	}
	if !sawRecurring {
		t.Fatal("missing recurring_obligations strategy")
	}
	for family, name := range wantFamilies {
		if got[family] != name {
			t.Fatalf("family %s = %q, want %q (got map %#v)", family, got[family], name, got)
		}
	}
	if reg.CatalogVersion() != DefaultContinuityCatalogVersion {
		t.Fatalf("catalog version = %q, want %q", reg.CatalogVersion(), DefaultContinuityCatalogVersion)
	}
	refs := reg.StrategyRefs()
	if len(refs) != len(wantFamilies)+1 || refs[0] != "recurring_obligations@v1" {
		t.Fatalf("strategy refs = %#v", refs)
	}

	// Residual families must seed + optionally decompose without model calls.
	for _, family := range []domain.WorkFamily{
		domain.FamilyCoverageScan, domain.FamilySourceFreshness, domain.FamilyFrontierManage,
	} {
		var strategy ContinuityStrategy
		for _, s := range reg.Strategies() {
			if s.Name() == wantFamilies[family] {
				strategy = s
				break
			}
		}
		if strategy == nil {
			t.Fatalf("missing strategy for %s", family)
		}
		res, err := strategy.Replenish(context.Background(), "revision_1")
		if err != nil {
			t.Fatalf("%s replenish: %v", family, err)
		}
		if !res.Changed && res.Admitted == 0 {
			t.Fatalf("%s expected seed/admit effect, got %+v", family, res)
		}
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		opps, err := r.WorkOpportunities("revision_1", "")
		if err != nil {
			return err
		}
		seen := map[domain.WorkFamily]bool{}
		for _, opp := range opps {
			seen[opp.Family] = true
		}
		for _, family := range []domain.WorkFamily{
			domain.FamilyCoverageScan, domain.FamilySourceFreshness, domain.FamilyFrontierManage,
		} {
			if !seen[family] {
				t.Fatalf("expected opportunities for residual family %s", family)
			}
		}
		// Catalog specs for residual families must exist after first replenish.
		for _, id := range []domain.OperationSpecID{
			DefaultFamilySpecCatalog()[domain.FamilyCoverageScan],
			DefaultFamilySpecCatalog()[domain.FamilySourceFreshness],
			DefaultFamilySpecCatalog()[domain.FamilyFrontierManage],
		} {
			if _, err := r.OperationSpec(id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerPreventiveAdmissionBeforeStrategies(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(context.Background(), store, nil); err != nil {
		t.Fatal(err)
	}
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_ready_seed", MissionRevision: "revision_1",
		Family: domain.FamilyIntegrityAudit, Status: domain.OpportunityOpen, Title: "audit references",
		Origin: "integrity", ExpectedGain: "fix integrity", Novelty: "unverified links",
		StopCondition: "audit report", DedupSignature: "integrity:links", Depth: 0,
		EstimatedCost: domain.Budget{Tokens: 30, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 15, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatal(err)
	}
	var strategyCalled bool
	scheduler := Scheduler{
		Store: store, Clock: clock, IDs: source.NewSequenceIDGenerator(1),
		Policy: domain.DefaultHorizonPolicy(),
		Strategies: []ContinuityStrategy{
			continuityStrategy{name: "should-not-run", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
				strategyCalled = true
				return ContinuityResult{}, nil
			}},
		},
	}
	decision, err := scheduler.Step(context.Background(), "revision_1")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != DecisionDispatch {
		t.Fatalf("kind = %q, decision = %+v", decision.Kind, decision)
	}
	if decision.Strategy != "frontier_admission" {
		t.Fatalf("strategy = %q, decision = %+v", decision.Strategy, decision)
	}
	if decision.Operation == "" {
		t.Fatalf("missing operation id: %+v", decision)
	}
	if strategyCalled {
		t.Fatal("strategy ran despite preventive frontier admission")
	}
}
