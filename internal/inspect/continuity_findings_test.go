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

func TestProjectContinuityFindingsFromArtifacts(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "cf", Purpose: "continuity findings", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	olderBody := map[string]any{
		"schema":                            "local-operation-audit-v1",
		"operation_id":                      "op_gap_old",
		"spec_id":                           "continuity.gap_scan.v1",
		"mission_revision_id":               "revision_1",
		"ready_count":                       0,
		"open_opportunities":                2,
		"verified_at":                       now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
		"family":                            "gap_scan",
		"sources_without_observation_count": 1,
		"claims_without_evidence_count":     0,
		"findings":                          []string{"gap:no_ready_work", "gap:open_candidates=2"},
	}
	newerBody := map[string]any{
		"schema":                              "local-operation-audit-v1",
		"operation_id":                        "op_coverage_new",
		"spec_id":                             "continuity.mission_coverage_scan.v1",
		"mission_revision_id":                 "revision_1",
		"ready_count":                         1,
		"open_opportunities":                  0,
		"verified_at":                         now.Add(-time.Minute).Format(time.RFC3339Nano),
		"family":                              "coverage_scan",
		"sources_without_observation_count":   3,
		"claims_without_evidence_count":       1,
		"sources_without_fragment_count":      2,
		"fragments_without_observation_count": 4,
		"findings":                            []string{"coverage:sources_without_observation=3", "coverage:claims_without_evidence=1"},
	}
	staleBody := map[string]any{
		"schema":              "local-operation-audit-v1",
		"operation_id":        "op_fresh_stale",
		"spec_id":             "continuity.source_freshness.v1",
		"mission_revision_id": "revision_1",
		"ready_count":         0,
		"open_opportunities":  0,
		"verified_at":         now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		"family":              "source_freshness",
		"aging_source_count":  5,
		"findings":            []string{"freshness:aging_sources=5"},
	}
	otherMissionBody := map[string]any{
		"schema":              "local-operation-audit-v1",
		"operation_id":        "op_other",
		"spec_id":             "continuity.integrity_audit.v1",
		"mission_revision_id": "revision_other",
		"verified_at":         now.Format(time.RFC3339Nano),
		"family":              "integrity_audit",
		"findings":            []string{"integrity:should_not_appear"},
	}
	mustAppendAudit := func(id, kind string, body map[string]any, stale bool) domain.KnowledgeArtifact {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return domain.KnowledgeArtifact{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            domain.ArtifactID(id),
			Kind:          kind,
			BaseCommitID:  domain.GenesisCommitID,
			Dependencies:  []string{"fixture:" + id},
			ContentRef:    "inline:json:local-operation-audit-v1",
			Content:       string(raw),
			Stale:         stale,
		}
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
			return err
		}
		for _, a := range []domain.KnowledgeArtifact{
			mustAppendAudit("artifact_gap_old", "gap_scan_report", olderBody, false),
			mustAppendAudit("artifact_coverage_new", "coverage_scan_report", newerBody, false),
			mustAppendAudit("artifact_fresh_stale", "source_freshness_report", staleBody, true),
			mustAppendAudit("artifact_other_mission", "integrity_audit_report", otherMissionBody, false),
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "artifact_noise", Kind: "synthesis_note",
				BaseCommitID: domain.GenesisCommitID, Dependencies: []string{"fixture:noise"},
				ContentRef: "inline:text", Content: "not an audit",
			},
		} {
			if err := tx.AppendKnowledgeArtifact(a); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var proj inspect.ContinuityFindingsProjection
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		proj, err = inspect.ProjectContinuityFindings(r, mission.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if proj.TotalReports != 3 {
		t.Fatalf("total_reports = %d want 3 (other mission excluded)", proj.TotalReports)
	}
	if proj.ActiveReports != 2 || proj.StaleReports != 1 {
		t.Fatalf("active=%d stale=%d", proj.ActiveReports, proj.StaleReports)
	}
	if proj.Latest == nil || proj.Latest.ArtifactID != "artifact_coverage_new" {
		t.Fatalf("latest = %#v", proj.Latest)
	}
	if proj.Latest.SourcesWithoutObs != 3 || proj.Latest.ClaimsWithoutEv != 1 {
		t.Fatalf("latest counters = %#v", proj.Latest)
	}
	if len(proj.Latest.Findings) < 1 || proj.Latest.Findings[0] != "coverage:sources_without_observation=3" {
		t.Fatalf("latest findings = %#v", proj.Latest.Findings)
	}
	// gap_scan, coverage_scan, source_freshness — three families
	if len(proj.LatestByFamily) != 3 {
		t.Fatalf("latest_by_family len = %d (%#v)", len(proj.LatestByFamily), proj.LatestByFamily)
	}
	// stale family still surfaces
	foundStale := false
	for _, row := range proj.LatestByFamily {
		if row.Family == "source_freshness" {
			foundStale = true
			if !row.Stale || row.AgingSourceCount != 5 {
				t.Fatalf("stale family row = %#v", row)
			}
		}
	}
	if !foundStale {
		t.Fatal("expected source_freshness family row")
	}

	// Overview embeds the projection.
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	overview, err := projector.BuildOverview(context.Background(), mission.MissionID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Mission == nil || overview.Mission.ContinuityFindings == nil {
		t.Fatalf("overview missing continuity_findings: %#v", overview.Mission)
	}
	if overview.Mission.ContinuityFindings.Latest == nil || overview.Mission.ContinuityFindings.Latest.Kind != "coverage_scan_report" {
		t.Fatalf("overview findings = %#v", overview.Mission.ContinuityFindings)
	}

	// HTTP endpoint.
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	bad := mustGET(t, server.URL+"/continuity/findings")
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing mission status = %d", bad.StatusCode)
	}

	okResp := mustGET(t, server.URL+"/continuity/findings?mission_id=mission_1")
	defer okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("findings status = %d", okResp.StatusCode)
	}
	var httpProj inspect.ContinuityFindingsProjection
	if err := json.NewDecoder(okResp.Body).Decode(&httpProj); err != nil {
		t.Fatal(err)
	}
	if httpProj.TotalReports != 3 || httpProj.Latest == nil {
		t.Fatalf("http projection = %#v", httpProj)
	}
}

func TestProjectContinuityFindingsRedactsSecrets(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	body := map[string]any{
		"schema":              "local-operation-audit-v1",
		"operation_id":        "op_sec",
		"spec_id":             "continuity.integrity_audit.v1",
		"mission_revision_id": "revision_sec",
		"verified_at":         now.Format(time.RFC3339Nano),
		"family":              "integrity_audit",
		"findings":            []string{"integrity:token sk-abcdefghijklmnopqrstuvwxyz0123456789 leak"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.AppendKnowledgeArtifact(domain.KnowledgeArtifact{
			SchemaVersion: domain.SchemaVersionV1, ID: "artifact_sec", Kind: "integrity_audit_report",
			BaseCommitID: domain.GenesisCommitID, Dependencies: []string{"fixture:sec"},
			ContentRef: "inline:json:local-operation-audit-v1", Content: string(raw),
		})
	}); err != nil {
		t.Fatal(err)
	}
	var proj inspect.ContinuityFindingsProjection
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		proj, err = inspect.ProjectContinuityFindings(r, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if proj.Latest == nil || len(proj.Latest.Findings) == 0 {
		t.Fatalf("proj = %#v", proj)
	}
	for _, line := range proj.Latest.Findings {
		if containsOpenAIKeyShape(line) {
			t.Fatalf("secret leaked in finding: %q", line)
		}
	}
}

func containsOpenAIKeyShape(s string) bool {
	// sk- followed by a long alnum run should not survive redaction.
	for i := 0; i+3 < len(s); i++ {
		if s[i] == 's' && s[i+1] == 'k' && s[i+2] == '-' {
			run := 0
			for j := i + 3; j < len(s); j++ {
				c := s[j]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
					run++
					continue
				}
				break
			}
			if run >= 20 {
				return true
			}
		}
	}
	return false
}
