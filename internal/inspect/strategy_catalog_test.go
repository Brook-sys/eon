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

func TestBuildContinuityStrategyCatalogStableAndCloneSafe(t *testing.T) {
	cat := inspect.BuildContinuityStrategyCatalog("continuity-catalog.v2", []inspect.ContinuityStrategyDescriptor{
		{Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v2", Priority: 30, LocalOnly: true},
		{Name: "integrity_audit", Family: domain.FamilyIntegrityAudit, Version: "v2", Priority: 20, LocalOnly: true, Ref: "integrity_audit@v2"},
	})
	if cat.SchemaVersion != domain.SchemaVersionV1 {
		t.Fatalf("schema = %d", cat.SchemaVersion)
	}
	if cat.CatalogVersion != "continuity-catalog.v2" || cat.StrategyCount != 2 {
		t.Fatalf("catalog = %#v", cat)
	}
	if cat.Strategies[0].Ref != "gap_scan@v2" || cat.StrategyRefs[1] != "integrity_audit@v2" {
		t.Fatalf("refs = %#v strategies=%#v", cat.StrategyRefs, cat.Strategies)
	}
	clone := cat.Clone()
	clone.Strategies[0].Name = "mutated"
	clone.StrategyRefs[0] = "mutated@x"
	if cat.Strategies[0].Name != "gap_scan" || cat.StrategyRefs[0] != "gap_scan@v2" {
		t.Fatal("clone shared underlying storage")
	}
}

func TestProjectorOverviewEmbedsContinuityCatalog(t *testing.T) {
	store, mission, _, now := seedRuntime(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	projector.SetContinuityCatalog(inspect.BuildContinuityStrategyCatalog("continuity-catalog.v2", []inspect.ContinuityStrategyDescriptor{
		{Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v2", Priority: 30, LocalOnly: true},
		{Name: "frontier_management", Family: domain.FamilyFrontierManage, Version: "v2", Priority: 16, LocalOnly: true},
	}))

	overview, err := projector.BuildOverview(context.Background(), mission.MissionID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.ContinuityCatalog == nil {
		t.Fatal("expected continuity_catalog on overview")
	}
	if overview.ContinuityCatalog.CatalogVersion != "continuity-catalog.v2" {
		t.Fatalf("catalog version = %q", overview.ContinuityCatalog.CatalogVersion)
	}
	if overview.ContinuityCatalog.StrategyCount != 2 || len(overview.ContinuityCatalog.StrategyRefs) != 2 {
		t.Fatalf("catalog = %#v", overview.ContinuityCatalog)
	}
	if overview.Mission == nil || overview.Mission.LatestDiagnosis == nil {
		t.Fatal("expected latest diagnosis from seed")
	}
	if overview.Mission.LatestDiagnosis.CatalogVersion != "continuity-catalog.v2" {
		t.Fatalf("diagnosis catalog_version = %q detail=%q",
			overview.Mission.LatestDiagnosis.CatalogVersion, overview.Mission.LatestDiagnosis.SafeDetail)
	}
	if overview.Mission.LatestDiagnosis.StrategiesTried[0] != "gap_scan@v2" {
		t.Fatalf("strategies_tried = %#v", overview.Mission.LatestDiagnosis.StrategiesTried)
	}
}

func TestContinuityCatalogHTTPAndVersion(t *testing.T) {
	store, _, _, now := seedRuntime(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor", Version: "v-test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	// Without catalogue configured → 404.
	missing := mustGET(t, server.URL+"/continuity/catalog")
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing catalog status = %d", missing.StatusCode)
	}

	projector.SetContinuityCatalog(inspect.BuildContinuityStrategyCatalog("continuity-catalog.v2", []inspect.ContinuityStrategyDescriptor{
		{Name: "gap_scan", Family: domain.FamilyGapScan, Version: "v2", Priority: 30, LocalOnly: true},
	}))

	okResp := mustGET(t, server.URL+"/continuity/catalog")
	defer okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("catalog status = %d", okResp.StatusCode)
	}
	var body inspect.ContinuityStrategyCatalog
	if err := json.NewDecoder(okResp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.CatalogVersion != "continuity-catalog.v2" || body.StrategyCount != 1 {
		t.Fatalf("http catalog = %#v", body)
	}
	if len(body.Strategies) != 1 || body.Strategies[0].Ref != "gap_scan@v2" {
		t.Fatalf("strategies = %#v", body.Strategies)
	}

	ver := mustGET(t, server.URL+"/version")
	defer ver.Body.Close()
	if ver.StatusCode != http.StatusOK {
		t.Fatalf("version status = %d", ver.StatusCode)
	}
	var version map[string]any
	if err := json.NewDecoder(ver.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	if version["continuity_catalog_version"] != "continuity-catalog.v2" {
		t.Fatalf("version payload = %#v", version)
	}
	if n, ok := version["continuity_strategy_count"].(float64); !ok || n != 1 {
		t.Fatalf("strategy count = %#v", version["continuity_strategy_count"])
	}

	// Overview also embeds catalogue.
	ov := mustGET(t, server.URL+"/overview")
	defer ov.Body.Close()
	var overview inspect.Overview
	if err := json.NewDecoder(ov.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if overview.ContinuityCatalog == nil || overview.ContinuityCatalog.CatalogVersion != "continuity-catalog.v2" {
		t.Fatalf("overview catalog = %#v", overview.ContinuityCatalog)
	}
}

func TestDiagnosisCatalogVersionExtractionEdgeCases(t *testing.T) {
	now := time.Date(2026, 7, 16, 19, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "rev_edge", MissionID: "mission_edge", Revision: 1,
		OriginalText: "edge", Purpose: "edge", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	cases := []struct {
		name   string
		detail string
		want   string
	}{
		{name: "plain", detail: "no work; catalog=continuity-catalog.v2", want: "continuity-catalog.v2"},
		{name: "trailing_sep", detail: "no work; catalog=continuity-catalog.v2; note=x", want: "continuity-catalog.v2"},
		{name: "absent", detail: "no ready work under policy", want: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := memory.New()
			if err := s.Update(context.Background(), func(tx port.Transaction) error {
				if err := tx.AppendMissionRevision(mission); err != nil {
					return err
				}
				if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
					return err
				}
				return tx.CreateContinuityDiagnosis(domain.ContinuityDiagnosis{
					SchemaVersion: domain.SchemaVersionV1, ID: domain.ContinuityDiagnosisID("diag_" + tc.name),
					MissionRevision: mission.ID, OccurredAt: now,
					StrategiesTried: []string{"gap_scan@v2"}, OpenCandidateCount: 0, ReadyCount: 0,
					RecoveryConditions: []string{"admit open opportunity"},
					SafeDetail:         tc.detail, PolicyVersion: "horizon.v1",
				})
			}); err != nil {
				t.Fatal(err)
			}
			projector, err := inspect.NewProjector(s, inspect.RuntimeIdentity{Name: "m", Version: "t"})
			if err != nil {
				t.Fatal(err)
			}
			projector.Clock = func() time.Time { return now }
			overview, err := projector.BuildOverview(context.Background(), mission.MissionID)
			if err != nil {
				t.Fatal(err)
			}
			if overview.Mission == nil || overview.Mission.LatestDiagnosis == nil {
				t.Fatal("missing diagnosis")
			}
			got := overview.Mission.LatestDiagnosis.CatalogVersion
			if got != tc.want {
				t.Fatalf("catalog_version = %q want %q (detail=%q)", got, tc.want, tc.detail)
			}
		})
	}
}
