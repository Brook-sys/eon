package domain

import (
	"testing"
	"time"
)

func sampleObligation(id string, cadence time.Duration, anti AntiRepetitionPolicy) RecurringObligation {
	return RecurringObligation{
		SchemaVersion:  SchemaVersionV1,
		ID:             id,
		Kind:           RecurringKindHarness,
		Title:          "offline harness evaluation",
		Cadence:        cadence,
		Budget:         Budget{Tokens: 32, Attempts: 1},
		DeltaCriterion: "new offline compile report or fixture change",
		AntiRepetition: anti,
		MaxPerWindow:   1,
		Priority:       18,
		Enabled:        true,
		Objective:      "keep cognitive-v1 compile green",
	}
}

func TestRecurringObligationValidateAndFamily(t *testing.T) {
	ob := sampleObligation("harness_daily", time.Hour, AntiRepSkipWithoutDelta)
	if err := ob.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if ob.EffectiveFamily() != FamilyHarnessEvaluation {
		t.Fatalf("family = %s", ob.EffectiveFamily())
	}
	ob.Kind = RecurringKind("nope")
	if err := ob.Validate(); err == nil {
		t.Fatal("expected invalid kind")
	}
	if err := ValidateRecurringObligations([]RecurringObligation{
		sampleObligation("a", time.Minute, AntiRepSkipWithoutDelta),
		sampleObligation("a", time.Minute, AntiRepSkipWithoutDelta),
	}); err == nil {
		t.Fatal("expected duplicate id rejection")
	}
}

func TestPlanRecurringSeedsCadenceAndAntiRepetition(t *testing.T) {
	// Cadence 1h; bucket is unix/3600.
	start := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	ob := sampleObligation("harness_hourly", time.Hour, AntiRepRequireStateChange)
	mission := MissionRevisionID("revision_1")

	plans, err := PlanRecurringSeeds([]RecurringObligation{ob}, nil, mission, start, "")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected one cadence seed, got %d", len(plans))
	}
	if plans[0].Reason != "cadence_due" {
		t.Fatalf("reason = %s", plans[0].Reason)
	}
	sig := plans[0].DedupSignature
	bucket := CadenceBucket(start, time.Hour)
	wantSig := RecurringDedupSignature(ob.ID, mission, bucket, "")
	if sig != wantSig {
		t.Fatalf("sig=%s want=%s", sig, wantSig)
	}

	// Same period with existing signature: no duplicate (anti-repetition).
	existing := []WorkOpportunity{{
		SchemaVersion: SchemaVersionV1, ID: "opp_1", MissionRevision: mission,
		Family: FamilyHarnessEvaluation, Status: OpportunityOpen,
		Title: "x", Origin: "t", ExpectedGain: "g", Novelty: "n", StopCondition: "s",
		DedupSignature: sig, Depth: 0, EstimatedCost: Budget{Attempts: 1}, Risk: RiskLow,
		Priority: 18, CreatedAt: start, UpdatedAt: start,
	}}
	plans, err = PlanRecurringSeeds([]RecurringObligation{ob}, existing, mission, start.Add(10*time.Minute), "")
	if err != nil {
		t.Fatalf("plan mid-window: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected no pre-cadence duplicate, got %+v", plans)
	}

	// Mid-window state fingerprint enables one delta seed.
	plans, err = PlanRecurringSeeds([]RecurringObligation{ob}, existing, mission, start.Add(15*time.Minute), "head_commit_2")
	if err != nil {
		t.Fatalf("plan delta: %v", err)
	}
	if len(plans) != 1 || plans[0].Reason != "state_delta" {
		t.Fatalf("expected state_delta plan, got %+v", plans)
	}
	deltaSig := plans[0].DedupSignature
	if deltaSig == sig {
		t.Fatal("delta signature must differ from base period signature")
	}

	// Same fingerprint again: no empty reseed.
	existing = append(existing, WorkOpportunity{
		SchemaVersion: SchemaVersionV1, ID: "opp_2", MissionRevision: mission,
		Family: FamilyHarnessEvaluation, Status: OpportunityOpen,
		Title: "x", Origin: "t", ExpectedGain: "g", Novelty: "n", StopCondition: "s",
		DedupSignature: deltaSig, Depth: 0, EstimatedCost: Budget{Attempts: 1}, Risk: RiskLow,
		Priority: 18, CreatedAt: start, UpdatedAt: start,
	})
	plans, err = PlanRecurringSeeds([]RecurringObligation{ob}, existing, mission, start.Add(20*time.Minute), "head_commit_2")
	if err != nil {
		t.Fatalf("plan same fp: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected no reseed for same fingerprint, got %+v", plans)
	}

	// Next cadence bucket: limited recurrence even without fingerprint.
	next := start.Add(time.Hour)
	plans, err = PlanRecurringSeeds([]RecurringObligation{ob}, existing, mission, next, "")
	if err != nil {
		t.Fatalf("plan next period: %v", err)
	}
	if len(plans) != 1 || plans[0].Reason != "cadence_due" {
		t.Fatalf("expected next period cadence seed, got %+v", plans)
	}
	if CadenceBucket(next, time.Hour) == bucket {
		t.Fatal("test clock must advance into a new bucket")
	}
	if plans[0].PeriodBucket != CadenceBucket(next, time.Hour) {
		t.Fatalf("period bucket = %d", plans[0].PeriodBucket)
	}
}

func TestPlanRecurringSeedsDisabledAndSkipWithoutDelta(t *testing.T) {
	start := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	ob := sampleObligation("gap_hourly", time.Hour, AntiRepSkipWithoutDelta)
	ob.Kind = RecurringKindGapScan
	mission := MissionRevisionID("revision_1")
	plans, err := PlanRecurringSeeds([]RecurringObligation{ob}, nil, mission, start, "fp1")
	if err != nil || len(plans) != 1 {
		t.Fatalf("initial: plans=%+v err=%v", plans, err)
	}
	existing := []WorkOpportunity{{
		SchemaVersion: SchemaVersionV1, ID: "opp_1", MissionRevision: mission,
		Family: FamilyGapScan, Status: OpportunityAdmitted,
		Title: "x", Origin: "t", ExpectedGain: "g", Novelty: "n", StopCondition: "s",
		DedupSignature: plans[0].DedupSignature, Depth: 0, EstimatedCost: Budget{Attempts: 1},
		Risk: RiskLow, Priority: 18, CreatedAt: start, UpdatedAt: start, AdmittedInquiryID: "inq_1",
	}}
	// skip_without_delta ignores fingerprint mid-window.
	plans, err = PlanRecurringSeeds([]RecurringObligation{ob}, existing, mission, start.Add(time.Minute), "fp2")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("skip_without_delta must not reseed mid-window, got %+v", plans)
	}
	ob.Enabled = false
	plans, err = PlanRecurringSeeds([]RecurringObligation{ob}, nil, mission, start.Add(2*time.Hour), "")
	if err != nil || len(plans) != 0 {
		t.Fatalf("disabled must yield no plans: %+v err=%v", plans, err)
	}
}

func TestMissionRevisionValidatesRecurring(t *testing.T) {
	m := baseMission()
	m.StandingObjectives = []string{"keep knowledge current"}
	m.RecurringObligations = []RecurringObligation{sampleObligation("harness", time.Hour, AntiRepSkipWithoutDelta)}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	m.RecurringObligations[0].Cadence = 0
	if err := m.Validate(); err == nil {
		t.Fatal("expected cadence validation error")
	}
}

func TestDiffIncludesRecurringObligations(t *testing.T) {
	base := baseMission()
	candidate := base
	candidate.ID = "candidate"
	candidate.Revision = 2
	candidate.Provenance = "candidate"
	candidate.RecurringObligations = []RecurringObligation{sampleObligation("harness", time.Hour, AntiRepSkipWithoutDelta)}
	diff, err := DiffMissionRevisions(base, candidate)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	found := false
	for _, c := range diff.Changes {
		if c.Path == "recurring_obligations" {
			found = true
			if c.After == "" {
				t.Fatal("after fingerprint should be non-empty")
			}
		}
	}
	if !found {
		t.Fatalf("diff missing recurring_obligations: %+v", diff.Changes)
	}
	impact, err := PreviewMissionImpact(base, candidate, diff)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	found = false
	for _, item := range impact.Items {
		if item.Reference == "recurring_obligations" {
			found = true
			if item.Disposition != ImpactNewCapability {
				t.Fatalf("disposition = %s", item.Disposition)
			}
		}
	}
	if !found {
		t.Fatalf("impact missing recurring item: %+v", impact.Items)
	}
}
