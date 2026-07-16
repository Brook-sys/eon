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

func TestLocalArtifactRefreshMarksStaleAgainstHead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	// Knowledge artifact at genesis becomes stale after head advances.
	seedArt := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1, ID: "artifact_seed_1", Kind: "cited_claim_view",
		BaseCommitID: domain.GenesisCommitID,
		Dependencies: []string{"claim:seed"},
		ContentRef:   "inline:seed",
		Content:      "seed knowledge body",
		Stale:        false,
	}
	// Local audit kind must remain non-stale even when base is genesis.
	auditArt := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1, ID: "artifact_audit_seed", Kind: "gap_scan_report",
		BaseCommitID: domain.GenesisCommitID,
		Dependencies: []string{"operation:past"},
		ContentRef:   "inline:json:local-operation-audit-v1",
		Content:      `{"schema":"local-operation-audit-v1"}`,
		Stale:        false,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.AppendKnowledgeArtifact(seedArt); err != nil {
			return err
		}
		if err := tx.AppendKnowledgeArtifact(auditArt); err != nil {
			return err
		}
		return seedHeadCommit(tx, "revision_1", "commit_head_1", now)
	}); err != nil {
		t.Fatalf("seed knowledge/head: %v", err)
	}

	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_refresh_1",
		MissionRevision: "revision_1", Family: domain.FamilyArtifactRefresh,
		Title: "refresh stale", Origin: "test", ExpectedGain: "stale marks",
		Novelty: "refresh1", StopCondition: "marked", DedupSignature: "refresh:test",
		Risk: domain.RiskLow, Priority: 15, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		Status: domain.OpportunityOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatal(err)
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: ids, Catalog: DefaultFamilySpecCatalog()}
	admitted, err := admitter.AdmitOne(ctx, opp.ID)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}
	result, err := exec.Execute(ctx, admitted.Operation.ID)
	if err != nil || !result.Completed {
		t.Fatalf("execute: err=%v result=%+v", err, result)
	}

	var marked domain.KnowledgeArtifact
	var audit domain.KnowledgeArtifact
	var report domain.KnowledgeArtifact
	if err := store.View(ctx, func(r port.Reader) error {
		var err error
		marked, err = r.KnowledgeArtifact(seedArt.ID)
		if err != nil {
			return err
		}
		audit, err = r.KnowledgeArtifact(auditArt.ID)
		if err != nil {
			return err
		}
		report, err = r.KnowledgeArtifact(result.ArtifactID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !marked.Stale {
		t.Fatalf("expected seed artifact marked stale, got %+v", marked)
	}
	if audit.Stale {
		t.Fatalf("audit kind must not be auto-staled: %+v", audit)
	}
	if report.Kind != "artifact_refresh_report" {
		t.Fatalf("report kind = %q", report.Kind)
	}
	for _, needle := range []string{
		`"family":"artifact_refresh"`,
		`"stale_artifacts_marked":1`,
		`refresh:marked_stale=artifact_seed_1`,
		`"head_commit_id":"commit_head_1"`,
	} {
		if !strings.Contains(report.Content, needle) {
			t.Fatalf("report missing %q in %s", needle, report.Content)
		}
	}
}

func TestLocalSourceFreshnessAgingFindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	old := now.Add(-10 * 24 * time.Hour)
	fresh := now.Add(-2 * time.Hour)
	if err := store.Update(ctx, func(tx port.Transaction) error {
		oldBody := []byte("old body")
		oldHash := "sha256:2e599d46723a6e7f099e12d2bd8f8b8d77a2e043fde6f9a9c8149b204360b2b2"
		oldSrc := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_old", Kind: "web", Locator: "https://example.old", ObservedAt: old}
		oldVer := domain.SourceVersion{
			SchemaVersion: domain.SchemaVersionV1, ID: "sv_old", SourceID: oldSrc.ID,
			ContentHash: oldHash, ContentRef: oldHash, ObservedAt: old,
		}
		oldSnap := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: oldVer.ID, MediaType: "text/plain", Content: oldBody}
		if err := tx.AppendSource(oldSrc, oldVer, oldSnap); err != nil {
			return err
		}
		freshBody := []byte("fresh body")
		freshHash := "sha256:3bff457899dd5b80afd9deb0a18950e73ca7c03ea1c21ba42ebc36d3030478bc"
		freshSrc := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_fresh", Kind: "web", Locator: "https://example.fresh", ObservedAt: fresh}
		freshVer := domain.SourceVersion{
			SchemaVersion: domain.SchemaVersionV1, ID: "sv_fresh", SourceID: freshSrc.ID,
			ContentHash: freshHash, ContentRef: freshHash, ObservedAt: fresh,
		}
		freshSnap := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: freshVer.ID, MediaType: "text/plain", Content: freshBody}
		return tx.AppendSource(freshSrc, freshVer, freshSnap)
	}); err != nil {
		t.Fatalf("seed sources: %v", err)
	}

	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_fresh_1",
		MissionRevision: "revision_1", Family: domain.FamilySourceFreshness,
		Title: "source age", Origin: "test", ExpectedGain: "aging list",
		Novelty: "fresh1", StopCondition: "listed", DedupSignature: "freshness:test",
		Risk: domain.RiskLow, Priority: 12, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		Status: domain.OpportunityOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatal(err)
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: ids, Catalog: DefaultFamilySpecCatalog()}
	admitted, err := admitter.AdmitOne(ctx, opp.ID)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}
	result, err := exec.Execute(ctx, admitted.Operation.ID)
	if err != nil || !result.Completed {
		t.Fatalf("execute: err=%v result=%+v", err, result)
	}

	var content string
	var kind string
	if err := store.View(ctx, func(r port.Reader) error {
		a, err := r.KnowledgeArtifact(result.ArtifactID)
		if err != nil {
			return err
		}
		content = a.Content
		kind = a.Kind
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if kind != "source_freshness_report" {
		t.Fatalf("kind = %q", kind)
	}
	for _, needle := range []string{
		`"family":"source_freshness_scan"`,
		`"aging_source_count":1`,
		`"freshness_max_age_hours":168`,
		`freshness:aging_source=source_old`,
		`freshness:aging_count=1`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("report missing %q in %s", needle, content)
		}
	}
	if strings.Contains(content, "freshness:aging_source=source_fresh") {
		t.Fatalf("fresh source should not be aging: %s", content)
	}
}

func TestLocalIntegrityAndConflictStructuralFindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()
	seedMission(t, store)
	if err := EnsureCatalogSpecs(ctx, store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	// Memory store rejects orphan anchors/bare claims at write time; the local
	// path still detects support+contradict pairs and unopposed claims that
	// can be constructed under normal ingestion.
	if err := store.Update(ctx, func(tx port.Transaction) error {
		body := []byte("body")
		hash := "sha256:230d8358dc8e8890b4c58deeb62912ee2f20357ae92a5cc861b98e68fe31acb5"
		src := domain.Source{SchemaVersion: domain.SchemaVersionV1, ID: "source_1", Kind: "fixture", Locator: "fixture.txt", ObservedAt: now}
		ver := domain.SourceVersion{
			SchemaVersion: domain.SchemaVersionV1, ID: "sv_1", SourceID: src.ID,
			ContentHash: hash, ContentRef: hash, ObservedAt: now,
		}
		snap := domain.SourceSnapshot{SchemaVersion: domain.SchemaVersionV1, SourceVersionID: ver.ID, MediaType: "text/plain", Content: body}
		frag := domain.SourceFragment{
			SchemaVersion: domain.SchemaVersionV1, ID: "frag_1", SourceVersionID: ver.ID,
			Location: "bytes:0-4", StartOffset: 0, EndOffset: 4, ContentHash: hash, ContentRef: hash,
		}
		obsOK := domain.Observation{
			SchemaVersion: domain.SchemaVersionV1, ID: "obs_ok", Statement: "ok", ExactQuote: "body",
			Anchor: domain.ObservationAnchor{SourceFragmentID: frag.ID}, Provenance: "test",
		}
		conflicted := domain.Claim{
			SchemaVersion: domain.SchemaVersionV1, ID: "claim_conflict", Proposition: "p", Qualifiers: map[string]string{"s": "t"}, Version: 1,
		}
		unopposed := domain.Claim{
			SchemaVersion: domain.SchemaVersionV1, ID: "claim_unopposed", Proposition: "q", Qualifiers: map[string]string{"s": "t"}, Version: 1,
		}
		support := domain.EvidenceLink{
			SchemaVersion: domain.SchemaVersionV1, ID: "ev_s", ObservationID: obsOK.ID, ClaimID: conflicted.ID,
			Relation: domain.EvidenceSupports, Rationale: "s",
		}
		contradict := domain.EvidenceLink{
			SchemaVersion: domain.SchemaVersionV1, ID: "ev_c", ObservationID: obsOK.ID, ClaimID: conflicted.ID,
			Relation: domain.EvidenceContradicts, Rationale: "c",
		}
		support2 := domain.EvidenceLink{
			SchemaVersion: domain.SchemaVersionV1, ID: "ev_u", ObservationID: obsOK.ID, ClaimID: unopposed.ID,
			Relation: domain.EvidenceSupports, Rationale: "u",
		}
		if err := tx.AppendSource(src, ver, snap); err != nil {
			return err
		}
		if err := tx.AppendSourceFragments(ver.ID, []domain.SourceFragment{frag}); err != nil {
			return err
		}
		if err := tx.AppendObservation(obsOK); err != nil {
			return err
		}
		if err := tx.AppendClaimWithEvidence(conflicted, []domain.EvidenceLink{support, contradict}); err != nil {
			return err
		}
		return tx.AppendClaimWithEvidence(unopposed, []domain.EvidenceLink{support2})
	}); err != nil {
		t.Fatalf("seed graph: %v", err)
	}

	// Integrity audit first.
	integrityOpp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_integrity_struct",
		MissionRevision: "revision_1", Family: domain.FamilyIntegrityAudit,
		Title: "integrity", Origin: "test", ExpectedGain: "findings", Novelty: "i1",
		StopCondition: "report", DedupSignature: "integrity:struct", Risk: domain.RiskLow, Priority: 5,
		EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1}, Status: domain.OpportunityOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	conflictOpp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_conflict_struct",
		MissionRevision: "revision_1", Family: domain.FamilyConflictReview,
		Title: "conflict", Origin: "test", ExpectedGain: "review", Novelty: "c1",
		StopCondition: "report", DedupSignature: "conflict:struct", Risk: domain.RiskLow, Priority: 6,
		EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1}, Status: domain.OpportunityOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateWorkOpportunity(integrityOpp); err != nil {
			return err
		}
		return tx.CreateWorkOpportunity(conflictOpp)
	}); err != nil {
		t.Fatal(err)
	}
	admitter := Admitter{Store: store, Clock: clock, IDs: ids, Catalog: DefaultFamilySpecCatalog()}
	exec := LocalExecutor{Store: store, Clock: clock, IDs: ids}

	admittedI, err := admitter.AdmitOne(ctx, integrityOpp.ID)
	if err != nil {
		t.Fatalf("admit integrity: %v", err)
	}
	resI, err := exec.Execute(ctx, admittedI.Operation.ID)
	if err != nil || !resI.Completed {
		t.Fatalf("integrity execute: err=%v result=%+v", err, resI)
	}
	var integrityContent string
	if err := store.View(ctx, func(r port.Reader) error {
		a, err := r.KnowledgeArtifact(resI.ArtifactID)
		if err != nil {
			return err
		}
		integrityContent = a.Content
		if a.Kind != "integrity_audit_report" {
			t.Fatalf("kind = %q", a.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`integrity:claim_with_support_and_contradict=claim_conflict`,
		`integrity:conflicted_claims=1`,
		`"family":"integrity_audit"`,
	} {
		if !strings.Contains(integrityContent, needle) {
			t.Fatalf("integrity missing %q in %s", needle, integrityContent)
		}
	}

	admittedC, err := admitter.AdmitOne(ctx, conflictOpp.ID)
	if err != nil {
		t.Fatalf("admit conflict: %v", err)
	}
	resC, err := exec.Execute(ctx, admittedC.Operation.ID)
	if err != nil || !resC.Completed {
		t.Fatalf("conflict execute: err=%v result=%+v", err, resC)
	}
	var conflictContent string
	if err := store.View(ctx, func(r port.Reader) error {
		a, err := r.KnowledgeArtifact(resC.ArtifactID)
		if err != nil {
			return err
		}
		conflictContent = a.Content
		if a.Kind != "conflict_review_report" {
			t.Fatalf("kind = %q", a.Kind)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`conflict:unopposed_supported_claims=1`,
		`conflict:claims_with_support_and_opposition=1`,
		`"family":"conflict_evidence_review"`,
	} {
		if !strings.Contains(conflictContent, needle) {
			t.Fatalf("conflict missing %q in %s", needle, conflictContent)
		}
	}
}

func TestApplyLocalFamilyEffectsStructuralOrphans(t *testing.T) {
	t.Parallel()
	// Direct unit path: store write APIs refuse orphans, but imported or
	// future backends may surface residual structural issues.
	now := time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC)
	obs := domain.Observation{
		SchemaVersion: domain.SchemaVersionV1, ID: "obs_1", Statement: "s", ExactQuote: "q",
		Anchor: domain.ObservationAnchor{SourceFragmentID: "frag_missing"}, Provenance: "t",
	}
	claim := domain.Claim{
		SchemaVersion: domain.SchemaVersionV1, ID: "claim_1", Proposition: "p",
		Qualifiers: map[string]string{"s": "t"}, Version: 1,
	}
	bare := domain.Claim{
		SchemaVersion: domain.SchemaVersionV1, ID: "claim_bare", Proposition: "b",
		Qualifiers: map[string]string{"s": "t"}, Version: 1,
	}
	linkOrphanObs := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1, ID: "ev_orphan_obs", ObservationID: "obs_missing",
		ClaimID: claim.ID, Relation: domain.EvidenceSupports,
	}
	linkOK := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1, ID: "ev_ok", ObservationID: obs.ID,
		ClaimID: claim.ID, Relation: domain.EvidenceSupports,
	}
	linkContra := domain.EvidenceLink{
		SchemaVersion: domain.SchemaVersionV1, ID: "ev_c", ObservationID: obs.ID,
		ClaimID: claim.ID, Relation: domain.EvidenceContradicts,
	}
	obsByID := map[domain.ObservationID]domain.Observation{obs.ID: obs}
	claimByID := map[domain.ClaimID]domain.Claim{claim.ID: claim, bare.ID: bare}
	effects, err := applyLocalFamilyEffects(
		nil,
		string(domain.FamilyIntegrityAudit),
		now,
		domain.GenesisCommitID,
		nil, nil, nil,
		map[domain.SourceVersionID]domain.SourceVersion{},
		map[domain.SourceFragmentID]domain.SourceFragment{},
		[]domain.Observation{obs},
		obsByID,
		[]domain.Claim{claim, bare},
		claimByID,
		[]domain.EvidenceLink{linkOrphanObs, linkOK, linkContra},
		0, 0, 0, 1, 0, 0, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if effects.OrphanEvidence != 1 {
		t.Fatalf("orphan evidence = %d", effects.OrphanEvidence)
	}
	if effects.OrphanObsAnchors != 1 {
		t.Fatalf("orphan obs anchors = %d", effects.OrphanObsAnchors)
	}
	joined := strings.Join(effects.Findings, "|")
	for _, needle := range []string{
		"integrity:orphan_evidence_links=1",
		"integrity:orphan_observation_fragment_anchors=1",
		"integrity:claim_with_support_and_contradict=claim_1",
		"integrity:claims_without_evidence=1",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("missing %q in %v", needle, effects.Findings)
		}
	}
}

// seedHeadCommit advances mission head via the full proposal → accept → apply path.
func seedHeadCommit(tx port.Transaction, mission domain.MissionRevisionID, commitID domain.CommitID, now time.Time) error {
	// Need a READY/terminal operation for raw model output binding.
	spec := ContinuityOperationSpec("continuity.integrity_audit@1", domain.AuthorityReadOnly)
	if err := tx.AppendOperationSpec(spec); err != nil && !errors.Is(err, port.ErrConflict) {
		return err
	}
	q := domain.Question{SchemaVersion: 1, ID: "q_head", MissionRevision: mission, Text: "head?", Origin: "fixture", Relevance: "test", AnswerCondition: "n/a"}
	cand := domain.InquiryCandidate{
		SchemaVersion: 1, ID: "cand_head", MissionRevision: mission, QuestionID: q.ID,
		DerivedFrom: []string{"fixture"}, ExpectedProgress: "head", Novelty: "h", Risk: domain.RiskLow,
		SourcePlan: []string{"fixture"}, AnswerCondition: "n/a", StopCondition: "done", ReviewAfter: now.Add(time.Hour),
	}
	inq := domain.Inquiry{
		SchemaVersion: 1, ID: "inq_head", CandidateID: cand.ID, MissionRevision: mission, QuestionID: q.ID,
		AdmissionReason: "fixture", StopCondition: "done", State: domain.StateSucceeded,
	}
	op := domain.Operation{
		SchemaVersion: 1, ID: "op_head", InquiryID: inq.ID, MissionRevision: mission, SpecID: spec.ID,
		ExpectedOutput: "report", IdempotencyKey: "idem_head", State: domain.StateSucceeded, Attempt: 1,
	}
	for _, step := range []error{
		tx.CreateQuestion(q),
		tx.CreateInquiryCandidate(cand),
		tx.CreateInquiry(inq),
		tx.CreateOperation(op),
	} {
		if step != nil {
			return step
		}
	}
	proposed := domain.ProposedChangeSet{
		SchemaVersion: domain.SchemaVersionV1, ID: "pcs_head", MissionRevision: mission,
		OperationID: op.ID, BaseCommitID: domain.GenesisCommitID, ReadSet: []string{"fixture"},
		Preconditions: []string{}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: "fixture", EntityID: "entity_head", PayloadRef: "payload_head"}},
		ExpectedDelta: "head advance", ValidatorIDs: []string{"schema"}, Provenance: "fixture", IdempotencyKey: "idem_head",
	}
	raw := domain.RawModelOutput{
		SchemaVersion: domain.SchemaVersionV1, ID: "raw_head", OperationID: op.ID,
		Model: "fixture", Content: `{}`, ContentHash: "hash_head", CreatedAt: now,
	}
	validation := domain.ValidationReceipt{
		SchemaVersion: domain.SchemaVersionV1, ID: "val_head", OperationID: op.ID,
		ChangeSetID: proposed.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID, ProducedAt: now,
	}
	accepted := domain.AcceptedChangeSet{
		SchemaVersion: domain.SchemaVersionV1, ID: "acs_head", ProposedChangeSetID: proposed.ID,
		ValidationReceiptIDs: []domain.ReceiptID{validation.ID}, AcceptedAt: now, PolicyVersion: "v1",
	}
	commit := domain.Commit{
		SchemaVersion: domain.SchemaVersionV1, ID: commitID, AcceptedChangeSetID: accepted.ID,
		MissionRevision: mission, BaseCommitID: domain.GenesisCommitID, Version: 1,
		CommittedAt: now, ReceiptID: "cr_head", IdempotencyKey: "idem_head",
	}
	receipt := domain.CommitReceipt{
		SchemaVersion: domain.SchemaVersionV1, ID: "cr_head", CommitID: commit.ID,
		ChangeSetID: accepted.ID, OperationID: op.ID, Version: 1, ProducedAt: now,
	}
	if err := tx.AppendRawModelOutput(raw); err != nil {
		return err
	}
	if err := tx.AppendProposedChangeSet(proposed); err != nil {
		return err
	}
	if err := tx.AppendValidationReceipt(validation); err != nil {
		return err
	}
	if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
		return err
	}
	return tx.ApplyCommit(commit, receipt, proposed.Changes)
}
