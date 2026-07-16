package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestFrontierListHygieneAndOpportunityInspector(t *testing.T) {
	store, mission, _, now := seedRuntime(t)
	// Expand reservoir: duplicate signature, over-depth child chain, deferred peer.
	// DefaultHorizonPolicy.MaxDepth is 3; child depth must equal parent+1 at create time.
	mid1 := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_mid1", MissionRevision: mission.ID,
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "mid depth 1",
		Origin: "fixture", ExpectedGain: "chain", Novelty: "lineage", StopCondition: "continue",
		DedupSignature: "inspect:frontier:mid1", Depth: 1, ParentID: "opp_inspect_gap",
		EstimatedCost: domain.Budget{Tokens: 5, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 4, CreatedAt: now, UpdatedAt: now,
	}
	mid2 := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_mid2", MissionRevision: mission.ID,
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "mid depth 2",
		Origin: "fixture", ExpectedGain: "chain", Novelty: "lineage", StopCondition: "continue",
		DedupSignature: "inspect:frontier:mid2", Depth: 2, ParentID: "opp_mid1",
		EstimatedCost: domain.Budget{Tokens: 5, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 4, CreatedAt: now, UpdatedAt: now,
	}
	mid3 := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_mid3", MissionRevision: mission.ID,
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "mid depth 3",
		Origin: "fixture", ExpectedGain: "chain", Novelty: "lineage", StopCondition: "continue",
		DedupSignature: "inspect:frontier:mid3", Depth: 3, ParentID: "opp_mid2",
		EstimatedCost: domain.Budget{Tokens: 5, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 4, CreatedAt: now, UpdatedAt: now,
	}
	deep := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_deep", MissionRevision: mission.ID,
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "too deep",
		Origin: "fixture", ExpectedGain: "trim", Novelty: "depth", StopCondition: "abandon",
		// Depth 4 exceeds DefaultHorizonPolicy.MaxDepth (3).
		DedupSignature: "inspect:frontier:deep", Depth: 4, ParentID: "opp_mid3",
		EstimatedCost: domain.Budget{Tokens: 5, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 3, CreatedAt: now, UpdatedAt: now,
	}
	// Note: store rejects two active opportunities with the same DedupSignature.
	// Signature merge/supersede is covered by pure domain planner tests; here we
	// exercise projection over a legal store snapshot (depth pressure + deferred).
	extra := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_coverage_extra", MissionRevision: mission.ID,
		Family: domain.FamilyCoverageScan, Status: domain.OpportunityOpen, Title: "coverage open",
		Origin: "fixture", ExpectedGain: "cover", Novelty: "unique", StopCondition: "done",
		DedupSignature: "inspect:frontier:coverage", Depth: 0,
		EstimatedCost: domain.Budget{Tokens: 5, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 5, CreatedAt: now.Add(-time.Minute), UpdatedAt: now,
	}
	deferred := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_deferred", MissionRevision: mission.ID,
		Family: domain.FamilyIntegrityAudit, Status: domain.OpportunityDeferred, Title: "parked",
		Origin: "fixture", ExpectedGain: "reopen later", Novelty: "deferred", StopCondition: "reopen",
		DedupSignature: "inspect:frontier:deferred", Depth: 0,
		EstimatedCost: domain.Budget{Tokens: 5, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 20, CreatedAt: now, UpdatedAt: now, AbandonReason: "max_candidates parking",
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for _, opp := range []domain.WorkOpportunity{mid1, mid2, mid3, deep, extra, deferred} {
			if err := tx.CreateWorkOpportunity(opp); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	// Overview hygiene signals.
	overview, err := projector.BuildOverview(context.Background(), mission.MissionID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Mission == nil || overview.Mission.Frontier == nil {
		t.Fatal("expected frontier on overview")
	}
	f := overview.Mission.Frontier
	// seed gap + mid1..mid3 + deep + extra = 6 open; deferred = 1
	if f.Open != 6 || f.Deferred != 1 {
		t.Fatalf("frontier counts open=%d deferred=%d total=%d", f.Open, f.Deferred, f.Total)
	}
	if f.UniqueSignatures < 6 {
		t.Fatalf("signature stats = unique=%d (store enforces active uniqueness)", f.UniqueSignatures)
	}
	if f.OverDepthOpen < 1 {
		t.Fatalf("expected over-depth open, got %d", f.OverDepthOpen)
	}
	if !f.NeedsHygiene {
		t.Fatal("expected needs_hygiene from dry-run")
	}
	if f.MaxCandidates <= 0 || f.PolicyVersion == "" {
		t.Fatalf("policy marks missing: %#v", f)
	}

	// List with family filter.
	page, err := projector.ListFrontier(context.Background(), mission.MissionID, inspect.FrontierListFilter{
		Family: domain.FamilyGapScan,
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total < 5 {
		t.Fatalf("gap_scan total = %d", page.Total)
	}
	for _, item := range page.Items {
		if item.Family != domain.FamilyGapScan {
			t.Fatalf("family filter leak: %#v", item)
		}
	}

	// Status filter DEFERRED.
	parked, err := projector.ListFrontier(context.Background(), mission.MissionID, inspect.FrontierListFilter{
		Status: domain.OpportunityDeferred,
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if parked.Total != 1 || len(parked.Items) != 1 || parked.Items[0].ID != "opp_deferred" {
		t.Fatalf("deferred page = %#v", parked)
	}

	// Opportunity inspector + lineage.
	detail, err := projector.OpportunityInspector(context.Background(), "opp_inspect_gap")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Opportunity.ID != "opp_inspect_gap" {
		t.Fatalf("detail = %#v", detail)
	}
	if detail.ChildrenCount < 1 {
		t.Fatalf("expected child (mid1), children=%d", detail.ChildrenCount)
	}

	// Hygiene dry-run: depth abandon (and possibly defer under default max_candidates).
	hygiene, err := projector.FrontierHygieneForMission(context.Background(), mission.MissionID)
	if err != nil {
		t.Fatal(err)
	}
	if !hygiene.NeedsCompact || hygiene.ActionCount < 1 {
		t.Fatalf("hygiene = %#v", hygiene)
	}
	if hygiene.AbandonedCount < 1 {
		t.Fatalf("expected depth abandon, got %#v", hygiene)
	}
	if len(hygiene.Findings) == 0 {
		t.Fatal("expected findings lines")
	}

	// HTTP surface.
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	listResp := mustGET(t, server.URL+"/frontier?mission_id=mission_1&status=OPEN&limit=50")
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listResp.StatusCode)
	}
	var listBody inspect.FrontierPage
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if listBody.Total < 3 {
		t.Fatalf("http list total = %d", listBody.Total)
	}

	hygResp := mustGET(t, server.URL+"/frontier/hygiene?mission_id=mission_1")
	defer hygResp.Body.Close()
	if hygResp.StatusCode != http.StatusOK {
		t.Fatalf("hygiene status = %d", hygResp.StatusCode)
	}
	var hygBody inspect.FrontierHygieneProjection
	if err := json.NewDecoder(hygResp.Body).Decode(&hygBody); err != nil {
		t.Fatal(err)
	}
	if hygBody.ActionCount < 1 || !hygBody.NeedsCompact {
		t.Fatalf("http hygiene = %#v", hygBody)
	}

	oppResp := mustGET(t, server.URL+"/frontier/opportunities/opp_inspect_gap")
	defer oppResp.Body.Close()
	if oppResp.StatusCode != http.StatusOK {
		t.Fatalf("opp status = %d", oppResp.StatusCode)
	}

	// Validation: missing mission_id.
	bad := mustGET(t, server.URL+"/frontier")
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing mission status = %d", bad.StatusCode)
	}

	// Invalid status.
	badStatus := mustGET(t, server.URL+"/frontier?mission_id=mission_1&status=NOPE")
	defer badStatus.Body.Close()
	if badStatus.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad status code = %d", badStatus.StatusCode)
	}

	// Read-only: POST not registered.
	post, err := http.Post(server.URL+"/frontier", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed && post.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected write status = %d", post.StatusCode)
	}

	// Tight MaxCandidates forces DEFER in dry-run via active horizon config.
	tight := domain.DefaultHorizonPolicy()
	tight.MaxCandidates = 1
	tight.Version = "horizon.tight.v1"
	hash, err := domain.ConfigPayloadHash(domain.ConfigScopeHorizon, nil, nil, &tight, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		draft := domain.ConfigDraft{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              "draft_horizon_tight",
			Scope:           domain.ConfigScopeHorizon,
			BasedOnRevision: 0,
			Applicability:   domain.ConfigNextCycle,
			Status:          domain.ConfigDraftOpen,
			ActorType:       domain.ActorOperator,
			ActorID:         "fixture",
			Reason:          "inspect hygiene pressure",
			Horizon:         &tight,
			CreatedAt:       now,
		}
		if err := tx.CreateConfigDraft(draft); err != nil {
			return err
		}
		rev := domain.ConfigRevision{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "cfg_horizon_tight",
			Scope:         domain.ConfigScopeHorizon,
			Revision:      1,
			Applicability: domain.ConfigNextCycle,
			ContentHash:   hash,
			ActorType:     domain.ActorOperator,
			ActorID:       "fixture",
			Reason:        "inspect hygiene pressure",
			DraftID:       draft.ID,
			Horizon:       &tight,
			AcceptedAt:    now,
		}
		if err := tx.AppendConfigRevision(rev); err != nil {
			return err
		}
		return tx.ActivateConfigRevision(domain.ConfigScopeHorizon, rev.ID)
	}); err != nil {
		t.Fatalf("tight horizon config wiring: %v", err)
	}
	pressed, err := projector.FrontierHygieneForMission(context.Background(), mission.MissionID)
	if err != nil {
		t.Fatal(err)
	}
	if pressed.PolicyVersion != tight.Version {
		t.Fatalf("policy version = %s want %s", pressed.PolicyVersion, tight.Version)
	}
	if pressed.MaxCandidates != 1 {
		t.Fatalf("max candidates = %d", pressed.MaxCandidates)
	}
	if pressed.DeferredCount < 1 && pressed.SupersededCount < 1 {
		t.Fatalf("expected pressure actions under max_candidates=1: %#v", pressed)
	}
}

func TestListFrontierRejectsUnknownFilters(t *testing.T) {
	store := memory.New()
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "n", Version: "v"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = projector.ListFrontier(context.Background(), "m1", inspect.FrontierListFilter{Status: "WAT"})
	if err == nil {
		t.Fatal("expected invalid status error")
	}
	_, err = projector.ListFrontier(context.Background(), "m1", inspect.FrontierListFilter{Family: "nope"})
	if err == nil {
		t.Fatal("expected invalid family error")
	}
}
