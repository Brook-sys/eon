package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func TestStrategyRegistryOrdersByPriorityAndRejectsDuplicates(t *testing.T) {
	reg := NewStrategyRegistry()
	low := continuityStrategy{name: "integrity_audit", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
		return ContinuityResult{}, nil
	}}
	high := continuityStrategy{name: "gap_scan", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
		return ContinuityResult{}, nil
	}}
	if err := reg.Register(StrategyDescriptor{
		Name: "integrity_audit", Family: domain.FamilyIntegrityAudit, Version: "v1", Priority: 10, LocalOnly: true,
	}, low); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(StrategyDescriptor{
		Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v1", Priority: 20,
	}, high); err != nil {
		t.Fatal(err)
	}
	names := reg.Strategies()
	if len(names) != 2 || names[0].Name() != "gap_scan" || names[1].Name() != "integrity_audit" {
		t.Fatalf("order = %#v", names)
	}
	if err := reg.Register(StrategyDescriptor{
		Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v2", Priority: 30,
	}, high); err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestPlanContinuityActionExpandThenDiagnose(t *testing.T) {
	horizon := domain.ExecutableHorizon{
		SchemaVersion: domain.SchemaVersionV1, MissionRevision: "revision_1", PolicyVersion: "horizon.v1",
		ReadyCount: 1, OpenCandidates: 0, TargetReady: 4, LowWatermark: 2, MaxReady: 8,
		ObservedAt: time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC),
	}
	plan, err := PlanContinuityAction(horizon, 2, "gap_scan")
	if err != nil || plan.Action != domain.ContinuityExpand || plan.Strategy != "gap_scan" {
		t.Fatalf("expand plan = %+v err=%v", plan, err)
	}
	plan, err = PlanContinuityAction(horizon, 0, "")
	if err != nil || plan.Action != domain.ContinuityDiagnose {
		t.Fatalf("diagnose plan = %+v err=%v", plan, err)
	}
	full := horizon
	full.ReadyCount = 3
	plan, err = PlanContinuityAction(full, 1, "integrity_audit")
	if err != nil || plan.Action != domain.ContinuityExpand || plan.Strategy != "integrity_audit" {
		t.Fatalf("expand when dispatch set empty = %+v err=%v", plan, err)
	}
}

func TestStrategyRegistrySnapshotAndRefs(t *testing.T) {
	reg := NewStrategyRegistry()
	gap := continuityStrategy{name: "gap_scan", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
		return ContinuityResult{}, nil
	}}
	integrity := continuityStrategy{name: "integrity_audit", run: func(context.Context, domain.MissionRevisionID) (ContinuityResult, error) {
		return ContinuityResult{}, nil
	}}
	if err := reg.Register(StrategyDescriptor{
		Name: "integrity_audit", Family: domain.FamilyIntegrityAudit, Version: "v2", Priority: 10, LocalOnly: true,
	}, integrity); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(StrategyDescriptor{
		Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v2", Priority: 30,
	}, gap); err != nil {
		t.Fatal(err)
	}
	reg.SetCatalogVersion(DefaultContinuityCatalogVersion)
	snap := reg.Snapshot()
	if snap.CatalogVersion != DefaultContinuityCatalogVersion {
		t.Fatalf("catalog version = %q", snap.CatalogVersion)
	}
	if len(snap.Descriptors) != 2 || snap.Descriptors[0].Name != "gap_scan" || snap.Descriptors[0].Version != "v2" {
		t.Fatalf("snapshot descriptors = %+v", snap.Descriptors)
	}
	refs := reg.StrategyRefs()
	if len(refs) != 2 || refs[0] != "gap_scan@v2" || refs[1] != "integrity_audit@v2" {
		t.Fatalf("refs = %#v", refs)
	}
	d, ok := reg.Descriptor("gap_scan")
	if !ok || d.Ref() != "gap_scan@v2" {
		t.Fatalf("descriptor = %+v ok=%v", d, ok)
	}
	// Mutation of snapshot must not mutate registry order/metadata.
	snap.Descriptors[0].Name = "mutated"
	if reg.Descriptors()[0].Name != "gap_scan" {
		t.Fatal("snapshot shared underlying descriptor storage")
	}
}

func TestCapChildDraftsRespectsMaxChildren(t *testing.T) {
	drafts := []ChildDraft{
		{Title: "a", Origin: "o", ExpectedGain: "g", Novelty: "n", StopCondition: "s", DedupSignature: "gap:a", Risk: domain.RiskLow, Priority: 3, EstimatedCost: domain.Budget{Tokens: 1, Attempts: 1}},
		{Title: "b", Origin: "o", ExpectedGain: "g", Novelty: "n", StopCondition: "s", DedupSignature: "gap:b", Risk: domain.RiskLow, Priority: 2, EstimatedCost: domain.Budget{Tokens: 1, Attempts: 1}},
		{Title: "c", Origin: "o", ExpectedGain: "g", Novelty: "n", StopCondition: "s", DedupSignature: "gap:c", Risk: domain.RiskLow, Priority: 1, EstimatedCost: domain.Budget{Tokens: 1, Attempts: 1}},
	}
	capped := capChildDrafts(drafts, 2)
	if len(capped) != 2 || capped[0].DedupSignature != "gap:a" || capped[1].DedupSignature != "gap:b" {
		t.Fatalf("capped = %+v", capped)
	}
	if capChildDrafts(drafts, 0) != nil {
		t.Fatal("max<=0 must yield nil")
	}
	if len(capChildDrafts(drafts, 10)) != 3 {
		t.Fatal("cap above length must preserve all")
	}
}
