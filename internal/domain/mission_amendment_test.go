package domain

import (
	"strings"
	"testing"
	"time"
)

func baseMission() MissionRevision {
	return MissionRevision{
		SchemaVersion: SchemaVersionV1,
		ID:            "mission_revision_1",
		MissionID:     "mission_1",
		Revision:      1,
		OriginalText:  "Investigate epistemic runtimes.",
		Purpose:       "Build cited knowledge",
		Domains:       []string{"runtime", "knowledge"},
		Policies:      []string{"cite", "no_model_authority"},
		Budget:        Budget{ModelCalls: 10, Tokens: 8000, Bytes: 65536, Attempts: 3, Duration: time.Hour},
		Status:        MissionActive,
		Provenance:    "user:initial",
		AcceptedAt:    time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
	}
}

func validAmendment() UserAmendment {
	return UserAmendment{
		SchemaVersion:     SchemaVersionV1,
		MissionID:         "mission_1",
		BaseRevision:      1,
		CandidateRevision: 2,
		OriginalText:      "Investigate epistemic runtimes and continuity.",
		Purpose:           "Build cited knowledge with durable continuity",
		Domains:           []string{"runtime", "knowledge", "continuity"},
		Policies:          []string{"cite", "no_model_authority"},
		Budget:            Budget{ModelCalls: 5, Tokens: 4000, Bytes: 65536, Attempts: 3, Duration: time.Hour},
		Status:            MissionActive,
		Reason:            "expand continuity scope and tighten model budget",
	}
}

func TestDiffAndImpactMissionAmendment(t *testing.T) {
	base := baseMission()
	amendment := validAmendment()
	candidate, err := CandidateFromAmendment(base, amendment)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	diff, err := DiffMissionRevisions(base, candidate)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diff.Empty || len(diff.Changes) == 0 {
		t.Fatalf("expected non-empty diff, got %#v", diff)
	}
	paths := map[string]bool{}
	for _, c := range diff.Changes {
		paths[c.Path] = true
	}
	for _, need := range []string{"original_text", "purpose", "domains", "budget.model_calls", "budget.tokens"} {
		if !paths[need] {
			t.Fatalf("missing change path %s in %#v", need, diff.Changes)
		}
	}

	impact, err := PreviewMissionImpact(base, candidate, diff)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if impact.Blocked || !impact.RequiresAcceptance {
		t.Fatalf("impact should require acceptance: %#v", impact)
	}
	var sawCancel, sawReprio, sawNew, sawKeep bool
	for _, item := range impact.Items {
		switch item.Disposition {
		case ImpactCancel:
			sawCancel = true
		case ImpactReprioritize:
			sawReprio = true
		case ImpactNewCapability:
			sawNew = true
		case ImpactKeep:
			sawKeep = true
		}
	}
	if !sawCancel || !sawReprio || !sawNew || !sawKeep {
		t.Fatalf("expected cancel/reprioritize/new_scope/keep dispositions, items=%#v", impact.Items)
	}
}

func TestMissionAmendmentRejectsNoopAndBadLineage(t *testing.T) {
	base := baseMission()
	noop := validAmendment()
	noop.OriginalText = base.OriginalText
	noop.Purpose = base.Purpose
	noop.Domains = append([]string(nil), base.Domains...)
	noop.Policies = append([]string(nil), base.Policies...)
	noop.Budget = base.Budget
	noop.Status = base.Status
	candidate, err := CandidateFromAmendment(base, noop)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	diff, err := DiffMissionRevisions(base, candidate)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !diff.Empty {
		t.Fatalf("expected empty diff, got %#v", diff.Changes)
	}
	impact, err := PreviewMissionImpact(base, candidate, diff)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if !impact.Blocked || impact.RequiresAcceptance {
		t.Fatalf("noop must be blocked: %#v", impact)
	}

	bad := validAmendment()
	bad.CandidateRevision = 3
	if err := bad.Validate(); err == nil {
		t.Fatal("non-monotonic candidate accepted")
	}

	badBase := validAmendment()
	badBase.BaseRevision = 9
	if _, err := CandidateFromAmendment(base, badBase); err == nil {
		t.Fatal("stale base revision accepted")
	}
}

func TestMissionDiffIsDeterministicAndSorted(t *testing.T) {
	base := baseMission()
	amendment := validAmendment()
	candidate, err := CandidateFromAmendment(base, amendment)
	if err != nil {
		t.Fatal(err)
	}
	d1, err := DiffMissionRevisions(base, candidate)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := DiffMissionRevisions(base, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(d1.Changes) != len(d2.Changes) {
		t.Fatalf("nondeterministic length")
	}
	for i := range d1.Changes {
		if d1.Changes[i] != d2.Changes[i] {
			t.Fatalf("nondeterministic order/content at %d: %#v vs %#v", i, d1.Changes[i], d2.Changes[i])
		}
		if i > 0 && d1.Changes[i-1].Path >= d1.Changes[i].Path {
			t.Fatalf("unsorted paths")
		}
	}
	// Domains set encoding must not depend on input order.
	scrambled := amendment
	scrambled.Domains = []string{"continuity", "knowledge", "runtime"}
	c2, err := CandidateFromAmendment(base, scrambled)
	if err != nil {
		t.Fatal(err)
	}
	d3, err := DiffMissionRevisions(base, c2)
	if err != nil {
		t.Fatal(err)
	}
	var domains1, domains3 string
	for _, c := range d1.Changes {
		if c.Path == "domains" {
			domains1 = c.After
		}
	}
	for _, c := range d3.Changes {
		if c.Path == "domains" {
			domains3 = c.After
		}
	}
	if domains1 == "" || domains1 != domains3 {
		t.Fatalf("domain encoding order-sensitive: %q vs %q", domains1, domains3)
	}
	if !strings.Contains(domains1, "continuity") {
		t.Fatalf("domains after missing continuity: %q", domains1)
	}
}
