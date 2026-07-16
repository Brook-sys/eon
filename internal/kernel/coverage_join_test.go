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

func TestCoverageJoinAndGapCoverageFamilyEffects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	// Mission with domains so coverage reports surface mission_domain findings.
	revision := domain.MissionRevision{
		SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "research", Purpose: "learn", Domains: []string{"epistemics", "sources"},
		Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now.Add(-time.Hour),
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.AppendMissionRevision(revision)
	}); err != nil {
		t.Fatalf("seed mission: %v", err)
	}
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	body := []byte("body")
	hash := "sha256:230d8358dc8e8890b4c58deeb62912ee2f20357ae92a5cc861b98e68fe31acb5"
	// source_covered: fragment + observation.
	// source_gap_frag: fragment without observation.
	// source_no_frag: source/version with no fragments.
	if err := store.Update(ctx, func(tx port.Transaction) error {
		covered := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_covered", Kind: "fixture", Locator: "covered.txt", ObservedAt: now}
		gapFrag := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_gap_frag", Kind: "fixture", Locator: "gap.txt", ObservedAt: now}
		noFrag := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_no_frag", Kind: "fixture", Locator: "empty.txt", ObservedAt: now}
		verC := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_covered", SourceID: covered.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		verG := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_gap", SourceID: gapFrag.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		verN := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_nofrag", SourceID: noFrag.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapC := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: verC.ID, MediaType: "text/plain", Content: body}
		snapG := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: verG.ID, MediaType: "text/plain", Content: body}
		snapN := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: verN.ID, MediaType: "text/plain", Content: body}
		fragC := domain.SourceFragment{SchemaVersion: domain.SchemaVersionV1, ID: "frag_covered", SourceVersionID: verC.ID, Location: "bytes:0-4", StartOffset: 0, EndOffset: 4, ContentHash: hash, ContentRef: hash}
		fragG := domain.SourceFragment{SchemaVersion: domain.SchemaVersionV1, ID: "frag_gap", SourceVersionID: verG.ID, Location: "bytes:0-4", StartOffset: 0, EndOffset: 4, ContentHash: hash, ContentRef: hash}
		obs := domain.Observation{
			SchemaVersion: domain.SchemaVersionV1, ID: "obs_covered", Statement: "covered", ExactQuote: "body",
			Anchor: domain.ObservationAnchor{SourceFragmentID: fragC.ID}, Provenance: "test",
		}
		for _, step := range []error{
			tx.AppendSource(covered, verC, snapC),
			tx.AppendSource(gapFrag, verG, snapG),
			tx.AppendSource(noFrag, verN, snapN),
			tx.AppendSourceFragments(verC.ID, []domain.SourceFragment{fragC}),
			tx.AppendSourceFragments(verG.ID, []domain.SourceFragment{fragG}),
			tx.AppendObservation(obs),
		} {
			if step != nil {
				return step
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed graph: %v", err)
	}

	// Unit: coverageJoin counts.
	var sources []domain.Source
	versionByID := map[domain.SourceVersionID]domain.SourceVersion{}
	fragmentByID := map[domain.SourceFragmentID]domain.SourceFragment{}
	var observations []domain.Observation
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		sources, err = r.Sources()
		if err != nil {
			return err
		}
		versions, err := r.SourceVersions("")
		if err != nil {
			return err
		}
		for _, ver := range versions {
			versionByID[ver.ID] = ver
			frags, fragErr := r.SourceFragments(ver.ID)
			if fragErr != nil {
				return fragErr
			}
			for _, frag := range frags {
				fragmentByID[frag.ID] = frag
			}
		}
		obs, err := r.Observations()
		if err != nil {
			return err
		}
		observations = obs
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	observed, withoutObs, withoutFrag, fragsWithoutObs := coverageJoin(sources, versionByID, fragmentByID, observations)
	if len(observed) != 1 {
		t.Fatalf("observed sources = %d, want 1", len(observed))
	}
	if _, ok := observed["source_covered"]; !ok {
		t.Fatalf("expected source_covered observed, got %#v", observed)
	}
	if withoutObs != 2 {
		t.Fatalf("withoutObs = %d, want 2", withoutObs)
	}
	if withoutFrag != 1 {
		t.Fatalf("withoutFrag = %d, want 1", withoutFrag)
	}
	if fragsWithoutObs != 1 {
		t.Fatalf("fragsWithoutObs = %d, want 1", fragsWithoutObs)
	}

	// Integration: gap + coverage local audits.
	gapOpp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_gap_join",
		MissionRevision: "revision_1", Family: domain.FamilyGapScan,
		Title: "gap", Origin: "test", ExpectedGain: "gaps", Novelty: "g1",
		StopCondition: "report", DedupSignature: "gap:join", Risk: domain.RiskLow, Priority: 5,
		EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1}, Status: domain.OpportunityOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	covOpp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_cov_join",
		MissionRevision: "revision_1", Family: domain.FamilyCoverageScan,
		Title: "coverage", Origin: "test", ExpectedGain: "cover", Novelty: "c1",
		StopCondition: "report", DedupSignature: "coverage:join", Risk: domain.RiskLow, Priority: 6,
		EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1}, Status: domain.OpportunityOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateWorkOpportunity(gapOpp); err != nil {
			return err
		}
		return tx.CreateWorkOpportunity(covOpp)
	}); err != nil {
		t.Fatal(err)
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: ids, Catalog: DefaultFamilySpecCatalog()}
	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}

	admittedG, err := admitter.AdmitOne(ctx, gapOpp.ID)
	if err != nil {
		t.Fatalf("admit gap: %v", err)
	}
	resG, err := exec.Execute(ctx, admittedG.Operation.ID)
	if err != nil || !resG.Completed {
		t.Fatalf("gap execute: err=%v result=%+v", err, resG)
	}
	var gapContent string
	if err := store.View(ctx, func(r port.Reader) error {
		a, err := r.KnowledgeArtifact(resG.ArtifactID)
		if err != nil {
			return err
		}
		gapContent = a.Content
		if a.Kind != "gap_scan_report" {
			t.Fatalf("kind = %q", a.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"gap:source_without_observation=source_gap_frag",
		"gap:source_without_observation=source_no_frag",
		"gap:sources_without_observation=2",
		"gap:sources_without_fragment=1",
		"gap:fragments_without_observation=1",
		`"sources_without_fragment_count":1`,
		`"fragments_without_observation_count":1`,
		`"family":"gap_scan"`,
	} {
		if !strings.Contains(gapContent, needle) {
			t.Fatalf("gap missing %q in %s", needle, gapContent)
		}
	}

	admittedC, err := admitter.AdmitOne(ctx, covOpp.ID)
	if err != nil {
		t.Fatalf("admit coverage: %v", err)
	}
	resC, err := exec.Execute(ctx, admittedC.Operation.ID)
	if err != nil || !resC.Completed {
		t.Fatalf("coverage execute: err=%v result=%+v", err, resC)
	}
	var covContent string
	if err := store.View(ctx, func(r port.Reader) error {
		a, err := r.KnowledgeArtifact(resC.ArtifactID)
		if err != nil {
			return err
		}
		covContent = a.Content
		if a.Kind != "coverage_scan_report" {
			t.Fatalf("kind = %q", a.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"coverage:mission_domains=2",
		"coverage:mission_domain=epistemics",
		"coverage:mission_domain=sources",
		"coverage:sources_without_observation=2",
		"coverage:sources_without_fragment=1",
		"coverage:fragments_without_observation=1",
		`"family":"mission_coverage_scan"`,
	} {
		if !strings.Contains(covContent, needle) {
			t.Fatalf("coverage missing %q in %s", needle, covContent)
		}
	}
}

func TestPlanChildDraftsFromStoreUsesJoins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 18, 30, 0, 0, time.UTC)
	store := memory.New()
	seedMission(t, store)
	body := []byte("body")
	hash := "sha256:230d8358dc8e8890b4c58deeb62912ee2f20357ae92a5cc861b98e68fe31acb5"
	if err := store.Update(ctx, func(tx port.Transaction) error {
		src := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_gap", Kind: "fixture", Locator: "gap.txt", ObservedAt: now.Add(-10 * 24 * time.Hour)}
		ver := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_gap", SourceID: src.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now.Add(-10 * 24 * time.Hour)}
		snap := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: ver.ID, MediaType: "text/plain", Content: body}
		frag := domain.SourceFragment{SchemaVersion: domain.SchemaVersionV1, ID: "frag_gap", SourceVersionID: ver.ID, Location: "bytes:0-4", StartOffset: 0, EndOffset: 4, ContentHash: hash, ContentRef: hash}
		if err := tx.AppendSource(src, ver, snap); err != nil {
			return err
		}
		return tx.AppendSourceFragments(ver.ID, []domain.SourceFragment{frag})
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	gapDrafts, err := PlanChildDraftsFromStore(ctx, store, domain.FamilyGapScan, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(gapDrafts) != 1 || gapDrafts[0].DedupSignature != "gap:join_inventory" {
		t.Fatalf("gap drafts = %+v", gapDrafts)
	}
	covDrafts, err := PlanChildDraftsFromStore(ctx, store, domain.FamilyCoverageScan, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(covDrafts) != 1 || covDrafts[0].DedupSignature != "coverage:join_inventory" {
		t.Fatalf("coverage drafts = %+v", covDrafts)
	}
	freshDrafts, err := PlanChildDraftsFromStore(ctx, store, domain.FamilySourceFreshness, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(freshDrafts) != 1 || freshDrafts[0].DedupSignature != "freshness:aging_sources" {
		t.Fatalf("freshness drafts = %+v", freshDrafts)
	}

	// Empty store / no gaps: fall back via resolveChildDrafts to static catalogue.
	empty := memory.New()
	seedMission(t, empty)
	resolved, err := resolveChildDrafts(ctx, empty, domain.FamilyGapScan, "revision_1", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].DedupSignature != "gap:scopes" {
		t.Fatalf("static fallback = %+v", resolved)
	}

	// integrity + conflict share a graph with support+contradict on one claim.
	structStore := memory.New()
	seedMission(t, structStore)
	if err := structStore.Update(ctx, func(tx port.Transaction) error {
		src := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_c", Kind: "fixture", Locator: "c.txt", ObservedAt: now}
		ver := domain.SourceVersion{SchemaVersion: domain.SchemaVersionV1, ID: "sv_c", SourceID: src.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snap := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: ver.ID, MediaType: "text/plain", Content: body}
		frag := domain.SourceFragment{SchemaVersion: domain.SchemaVersionV1, ID: "frag_c", SourceVersionID: ver.ID, Location: "bytes:0-4", StartOffset: 0, EndOffset: 4, ContentHash: hash, ContentRef: hash}
		obs := domain.Observation{
			SchemaVersion: domain.SchemaVersionV1, ID: "obs_c", Statement: "observed", ExactQuote: "body",
			Anchor: domain.ObservationAnchor{SourceFragmentID: frag.ID}, Provenance: "test",
		}
		claim := domain.Claim{
			SchemaVersion: domain.SchemaVersionV1, ID: "claim_conflicted", Proposition: "maybe",
			Qualifiers: map[string]string{"stance": "contested"}, Version: 1,
		}
		links := []domain.EvidenceLink{
			{SchemaVersion: domain.SchemaVersionV1, ID: "ev_support", ClaimID: claim.ID, ObservationID: obs.ID, Relation: domain.EvidenceSupports, Rationale: "s"},
			{SchemaVersion: domain.SchemaVersionV1, ID: "ev_contra", ClaimID: claim.ID, ObservationID: obs.ID, Relation: domain.EvidenceContradicts, Rationale: "c"},
		}
		for _, step := range []error{
			tx.AppendSource(src, ver, snap),
			tx.AppendSourceFragments(ver.ID, []domain.SourceFragment{frag}),
			tx.AppendObservation(obs),
			tx.AppendClaimWithEvidence(claim, links),
		} {
			if step != nil {
				return step
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed structural graph: %v", err)
	}
	integrityDrafts, err := PlanChildDraftsFromStore(ctx, structStore, domain.FamilyIntegrityAudit, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(integrityDrafts) != 1 || integrityDrafts[0].DedupSignature != "integrity:structural_inventory" {
		t.Fatalf("integrity drafts = %+v", integrityDrafts)
	}
	if !strings.Contains(integrityDrafts[0].ExpectedGain, "conflicted=1") {
		t.Fatalf("integrity gain = %q", integrityDrafts[0].ExpectedGain)
	}
	conflictDrafts, err := PlanChildDraftsFromStore(ctx, structStore, domain.FamilyConflictReview, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflictDrafts) != 1 || conflictDrafts[0].DedupSignature != "conflict:evidence_inventory" {
		t.Fatalf("conflict drafts = %+v", conflictDrafts)
	}
	if !strings.Contains(conflictDrafts[0].ExpectedGain, "conflicted=1") {
		t.Fatalf("conflict gain = %q", conflictDrafts[0].ExpectedGain)
	}

	// Clean graph: integrity/conflict planners stay silent so static fallback can apply.
	clean := memory.New()
	seedMission(t, clean)
	cleanIntegrity, err := PlanChildDraftsFromStore(ctx, clean, domain.FamilyIntegrityAudit, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanIntegrity) != 0 {
		t.Fatalf("clean integrity drafts = %+v", cleanIntegrity)
	}
	cleanConflict, err := PlanChildDraftsFromStore(ctx, clean, domain.FamilyConflictReview, "revision_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanConflict) != 0 {
		t.Fatalf("clean conflict drafts = %+v", cleanConflict)
	}
	// resolveChildDrafts falls back to static conflict catalogue on clean store.
	resolvedConflict, err := resolveChildDrafts(ctx, clean, domain.FamilyConflictReview, "revision_1", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvedConflict) != 1 || resolvedConflict[0].DedupSignature != "conflict:unopposed" {
		t.Fatalf("static conflict fallback = %+v", resolvedConflict)
	}
}
