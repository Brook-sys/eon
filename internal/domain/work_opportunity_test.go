package domain

import (
	"testing"
	"time"
)

func TestHorizonPolicyMarksAndReplenishment(t *testing.T) {
	policy := DefaultHorizonPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if !policy.NeedsReplenishment(2) || policy.NeedsReplenishment(3) {
		t.Fatalf("low watermark behaviour unexpected for policy %+v", policy)
	}
	if !policy.AcceptsAdmission(7) || policy.AcceptsAdmission(8) {
		t.Fatalf("max ready gate unexpected for policy %+v", policy)
	}
	bad := policy
	bad.LowWatermark = 9
	if err := bad.Validate(); err == nil {
		t.Fatal("expected invalid low_watermark ordering")
	}
}

func TestWorkOpportunityParentChildDerivation(t *testing.T) {
	now := time.Date(2026, 7, 16, 6, 0, 0, 0, time.UTC)
	root := WorkOpportunity{
		SchemaVersion: SchemaVersionV1, ID: "opp_root", MissionRevision: "revision_1",
		Family: FamilyGapScan, Status: OpportunityOpen, Title: "cover gaps", Origin: "mission",
		ExpectedGain: "new inquiries", Novelty: "uncovered scopes", StopCondition: "coverage target",
		DedupSignature: "gap:root", Depth: 0, EstimatedCost: Budget{Tokens: 10}, Risk: RiskLow,
		Priority: 10, CreatedAt: now, UpdatedAt: now,
	}
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}
	policy := DefaultHorizonPolicy()
	if err := root.CanSpawnChild(policy, 0); err != nil {
		t.Fatal(err)
	}
	child, err := root.DeriveChild("opp_child", "define term X", "decompose:root", "definition", "term undefined", "definition accepted", "gap:term:x", RiskLow, 8, now.Add(time.Minute), Budget{Tokens: 5})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != root.ID || child.Depth != 1 || child.Family != root.Family {
		t.Fatalf("child lineage = %+v", child)
	}
	if err := root.CanSpawnChild(policy, policy.MaxChildren); err == nil {
		t.Fatal("expected fan-out rejection")
	}
	deep := root
	deep.Depth = policy.MaxDepth
	deep.ParentID = "opp_mid"
	if err := deep.CanSpawnChild(policy, 0); err == nil {
		t.Fatal("expected depth rejection")
	}
}

func TestContinuityDiagnosisRequiresRecoveryPath(t *testing.T) {
	now := time.Date(2026, 7, 16, 6, 5, 0, 0, time.UTC)
	diag := ContinuityDiagnosis{
		SchemaVersion: SchemaVersionV1, ID: "diag_1", MissionRevision: "revision_1", OccurredAt: now,
		StrategiesTried: []string{"gap_scan", "integrity_audit"}, OpenCandidateCount: 0, ReadyCount: 0,
		RecoveryConditions: []string{"new authorized source", "operator clarification"},
		SafeDetail:         "no executable work after strategies", PolicyVersion: "horizon.v1",
	}
	if err := diag.Validate(); err != nil {
		t.Fatal(err)
	}
	diag.RecoveryConditions = nil
	if err := diag.Validate(); err == nil {
		t.Fatal("expected recovery conditions")
	}
}

func TestExecutableHorizonObservation(t *testing.T) {
	h := ExecutableHorizon{
		SchemaVersion: SchemaVersionV1, MissionRevision: "revision_1", PolicyVersion: "horizon.v1",
		ReadyCount: 1, OpenCandidates: 3, TargetReady: 4, LowWatermark: 2, MaxReady: 8,
		ObservedAt: time.Date(2026, 7, 16, 6, 10, 0, 0, time.UTC),
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if !h.NeedsReplenishment() {
		t.Fatal("expected replenishment at ready=1")
	}
}
