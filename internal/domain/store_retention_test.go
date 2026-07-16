package domain

import "testing"

func TestStoreRetentionPolicyDisallowsEventLogPrune(t *testing.T) {
	t.Parallel()
	p := DefaultStoreRetentionPolicy()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.IsRetentionActionAuthorized(RetentionActionEventLogPrune) {
		t.Fatal("event log prune must not be authorized")
	}
	for _, kind := range []RetentionActionKind{
		RetentionActionRefreshCandidates, RetentionActionFrontierHygiene, RetentionActionExportBufferTrim,
	} {
		if !p.IsRetentionActionAuthorized(kind) {
			t.Fatalf("expected %s authorized", kind)
		}
	}
	bad := p
	bad.AllowEventLogPrune = true
	if err := bad.Validate(); err == nil {
		t.Fatal("expected validate error when prune enabled")
	}
	normalized := bad.Normalize()
	if normalized.AllowEventLogPrune {
		t.Fatal("normalize must force prune off")
	}
}

func TestEventHeadAndStalePressureThresholds(t *testing.T) {
	t.Parallel()
	p := DefaultStoreRetentionPolicy()
	if p.EventHeadPressure(0) != "" {
		t.Fatal("zero head should be quiet")
	}
	if p.EventHeadPressure(DefaultEventHeadInfoSequence) != "info" {
		t.Fatal("want info at info threshold")
	}
	if p.EventHeadPressure(DefaultEventHeadWarnSequence) != "warn" {
		t.Fatal("want warn at warn threshold")
	}
	if p.StaleArtifactPressure(DefaultStaleArtifactInfoCount) != "info" {
		t.Fatal("want info stale pressure")
	}
	if p.StaleArtifactPressure(DefaultStaleArtifactWarnCount) != "warn" {
		t.Fatal("want warn stale pressure")
	}
}

func TestPlanStaleArtifactRefreshSkipsAuditAndCaps(t *testing.T) {
	t.Parallel()
	artifacts := []KnowledgeArtifact{
		{SchemaVersion: SchemaVersionV1, ID: "b", Kind: "cited_claim_view", BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:x@1"}, ContentRef: "sha256:b", Content: "b", Stale: true},
		{SchemaVersion: SchemaVersionV1, ID: "a", Kind: "cited_claim_view", BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:x@1"}, ContentRef: "sha256:a", Content: "a", Stale: true},
		{SchemaVersion: SchemaVersionV1, ID: "audit", Kind: "gap_scan_report", BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:x@1"}, ContentRef: "sha256:c", Content: "c", Stale: true},
		{SchemaVersion: SchemaVersionV1, ID: "fresh", Kind: "cited_claim_view", BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:y@1"}, ContentRef: "sha256:d", Content: "d", Stale: false},
	}
	ids := PlanStaleArtifactRefresh(artifacts, 1)
	if len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("want [a], got %v", ids)
	}
	ids = PlanStaleArtifactRefresh(artifacts, 10)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("want [a b], got %v", ids)
	}
}
