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
