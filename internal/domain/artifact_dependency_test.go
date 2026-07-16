package domain

import "testing"

func TestFormatAndParseClaimDependency(t *testing.T) {
	t.Parallel()
	ref := FormatClaimDependency("claim_1", 3)
	if ref != "claim:claim_1@3" {
		t.Fatalf("format: got %q", ref)
	}
	kind, id, version, ok := ParseDependencyRef(ref)
	if !ok || kind != DependencyKindClaim || id != "claim_1" || version != 3 {
		t.Fatalf("parse: kind=%q id=%q version=%d ok=%v", kind, id, version, ok)
	}
	if DependencyKey(ref) != "claim:claim_1" {
		t.Fatalf("key: got %q", DependencyKey(ref))
	}
}

func TestArtifactDependsOnMatchesVersionedClaim(t *testing.T) {
	t.Parallel()
	artifact := KnowledgeArtifact{
		SchemaVersion: SchemaVersionV1,
		ID:            "artifact_1",
		Kind:          "cited_claim_view",
		BaseCommitID:  GenesisCommitID,
		Dependencies:  []string{FormatClaimDependency("claim_1", 1), "evidence_link:ev_1"},
		ContentRef:    "sha256:x",
		Content:       "body",
	}
	if !ArtifactDependsOn(artifact, FormatDependency(DependencyKindClaim, "claim_1")) {
		t.Fatal("expected unversioned claim key to match versioned dependency")
	}
	if ArtifactDependsOn(artifact, FormatDependency(DependencyKindClaim, "claim_other")) {
		t.Fatal("unexpected match for other claim")
	}
	stale := artifact
	stale.Stale = true
	if ArtifactDependsOn(stale, FormatDependency(DependencyKindClaim, "claim_1")) {
		t.Fatal("stale artifacts must not re-match for cascade")
	}
}

func TestPlanArtifactInvalidationDeterministicAndSkipsAudit(t *testing.T) {
	t.Parallel()
	artifacts := []KnowledgeArtifact{
		{
			SchemaVersion: SchemaVersionV1, ID: "artifact_b", Kind: "cited_claim_view",
			BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:claim_1@1"},
			ContentRef: "sha256:b", Content: "b",
		},
		{
			SchemaVersion: SchemaVersionV1, ID: "artifact_a", Kind: "cited_claim_view",
			BaseCommitID: GenesisCommitID, Dependencies: []string{"observation:obs_1"},
			ContentRef: "sha256:a", Content: "a",
		},
		{
			SchemaVersion: SchemaVersionV1, ID: "artifact_audit", Kind: "gap_scan_report",
			BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:claim_1@1"},
			ContentRef: "sha256:c", Content: "c",
		},
		{
			SchemaVersion: SchemaVersionV1, ID: "artifact_stale", Kind: "cited_claim_view",
			BaseCommitID: GenesisCommitID, Dependencies: []string{"claim:claim_1@1"},
			ContentRef: "sha256:d", Content: "d", Stale: true,
		},
	}
	ids := PlanArtifactInvalidation(artifacts, []string{"claim:claim_1"}, IsLocalAuditArtifactKind)
	if len(ids) != 1 || ids[0] != "artifact_b" {
		t.Fatalf("want only artifact_b, got %v", ids)
	}
	// Observation-only change marks artifact_a only.
	ids = PlanArtifactInvalidation(artifacts, []string{"observation:obs_1"}, IsLocalAuditArtifactKind)
	if len(ids) != 1 || ids[0] != "artifact_a" {
		t.Fatalf("want only artifact_a, got %v", ids)
	}
}

func TestChangeAndEvidenceDependencyKeys(t *testing.T) {
	t.Parallel()
	keys := ChangeDependencyKeys([]Change{
		{Kind: ChangeReplace, EntityType: "claim", EntityID: "claim_1", PayloadRef: "p"},
		{Kind: ChangeAdd, EntityType: "observation", EntityID: "obs_1", PayloadRef: "p2"},
	})
	wantContains := []string{"claim:claim_1", "observation:obs_1", "canonical:claim/claim_1"}
	for _, want := range wantContains {
		found := false
		for _, got := range keys {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing key %q in %v", want, keys)
		}
	}
	evKeys := EvidenceDeltaDependencyKeys("claim_1", []EvidenceLink{
		{SchemaVersion: SchemaVersionV1, ID: "ev_2", ObservationID: "obs_2", ClaimID: "claim_1", Relation: EvidenceSupports},
	})
	for _, want := range []string{"claim:claim_1", "evidence_link:ev_2"} {
		found := false
		for _, got := range evKeys {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing evidence key %q in %v", want, evKeys)
		}
	}
	for _, got := range evKeys {
		if got == "observation:obs_2" {
			t.Fatalf("observation keys must not cascade on evidence-only deltas: %v", evKeys)
		}
	}
}
