package dashboard_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/dashboard"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestDashboardServesIndexAndProxiesAPIs(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "dash", Purpose: "dashboard test", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: "event_1", Kind: "mission.revision_activated",
			OccurredAt: now, MissionRevision: mission.ID, PayloadRef: string(mission.ID),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	inspectAPI, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	cmdInbox, err := control.NewCommandInbox(store, control.FixedReceiptFactory("receipt_1", now))
	if err != nil {
		t.Fatal(err)
	}
	evtInbox, err := control.NewExternalEventInbox(store, control.FixedDispositionFactory(now))
	if err != nil {
		t.Fatal(err)
	}
	controlAPI, err := control.NewAPI(cmdInbox, evtInbox, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	applier, err := kernel.NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	controlAPI.ConfigValidate = applier
	controlAPI.ConfigApply = applier
	ui, err := dashboard.New(inspectAPI.Handler(), controlAPI.Handler())
	if err != nil {
		t.Fatal(err)
	}
	ui.DefaultMissionID = string(mission.MissionID)
	server := httptest.NewServer(ui.Handler())
	t.Cleanup(server.Close)

	// Index HTML.
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "operator dashboard") || !strings.Contains(html, "API_BASE") || !strings.Contains(html, "/inspect") {
		t.Fatalf("index missing expected markers: %s", truncate(html, 400))
	}
	if !strings.Contains(html, "mission_1") {
		t.Fatal("default mission not prefilled")
	}
	for _, marker := range []string{
		"Configuração versionada",
		"/config/drafts",
		"/config/revisions/rollback",
		"Rollback semântico",
		"cfgRevisions",
		"PAUSE_MISSION",
		"btnCfgCreate",
		"credential_ref",
		"Inspetor de execução",
		"btnInspLoad",
		"/operations/",
		"raw_model_outputs",
		"open_candidates",
		"frontier_families",
		"frontier_hygiene",
		"needs_hygiene",
		"unique_signatures",
		"Frontier / higiene",
		"btnFrontList",
		"btnFrontHygiene",
		"btnFrontDetail",
		"/frontier/hygiene",
		"/frontier/opportunities/",
		"continuity_blocked",
		"continuity_catalog",
		"strategy_refs",
		"strategies_tried",
		"continuity_findings",
		"latest_audit",
		"latest_audit_links",
		"audits_by_family",
		"data-know-kind",
		"Abrir artifact",
		"Conhecimento",
		"/knowledge",
		"btnKnowList",
		"claims_without_evidence",
		"Commits / provider",
		"btnCommitList",
		"btnProviderProfile",
		"btnProviderProbe",
		"/commits",
		"/provider/profile",
		"FR-MODEL-005",
		"Alertas / telemetria",
		"btnAlertsRefresh",
		"btnTelemetry",
		"non-canonical",
		"/alerts",
		"/telemetry",
		"FR-CTRL-007",
		"Emenda de missão (FR-AUTH-004)",
		"/missions/amendments/preview",
		"/missions/amendments/accept",
		"btnAmendPreview",
		"btnAmendAccept",
		"btnAmendLoad",
		"standing_objectives",
		"recurring_obligations",
		"FR-DUR-011",
		"amendStanding",
		"amendRecurring",
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("index missing marker %q", marker)
		}
	}

	// Inspect proxied.
	ov, err := http.Get(server.URL + "/api/inspect/overview?mission_id=mission_1")
	if err != nil {
		t.Fatal(err)
	}
	defer ov.Body.Close()
	if ov.StatusCode != http.StatusOK {
		t.Fatalf("overview status = %d", ov.StatusCode)
	}
	var overview inspect.Overview
	if err := json.NewDecoder(ov.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if overview.Mission == nil || overview.Mission.MissionID != mission.MissionID {
		t.Fatalf("overview = %#v", overview)
	}

	// Control questions list proxied (empty pending is fine).
	q, err := http.Get(server.URL + "/api/control/questions?mission_id=mission_1")
	if err != nil {
		t.Fatal(err)
	}
	defer q.Body.Close()
	if q.StatusCode != http.StatusOK {
		t.Fatalf("questions status = %d", q.StatusCode)
	}

	// Config drafts list/create proxied through the dashboard mount.
	list, err := http.Get(server.URL + "/api/control/config/drafts?scope=INTERRUPTION")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	if list.StatusCode != http.StatusOK {
		t.Fatalf("config drafts status = %d", list.StatusCode)
	}
	policy := domain.DefaultInterruptionRuntimePolicy()
	policy.MaxPending = 7
	createBody, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"scope":          "INTERRUPTION",
		"reason":         "dashboard proxy draft",
		"interruption":   policy,
	})
	created, err := http.Post(server.URL+"/api/control/config/drafts", "application/json", strings.NewReader(string(createBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer created.Body.Close()
	if created.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(created.Body)
		t.Fatalf("create draft status = %d body=%s", created.StatusCode, body)
	}
	var createResp struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createResp); err != nil {
		t.Fatal(err)
	}
	if createResp.Draft.ID == "" || createResp.Draft.Status != domain.ConfigDraftOpen {
		t.Fatalf("create draft = %#v", createResp.Draft)
	}
	validated, err := http.Post(server.URL+"/api/control/config/drafts/"+string(createResp.Draft.ID)+"/validate", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer validated.Body.Close()
	if validated.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(validated.Body)
		t.Fatalf("validate draft status = %d body=%s", validated.StatusCode, body)
	}

	// SSE route is reachable through the dashboard mount.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/inspect/events/stream?poll_ms=50&limit=5", nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", stream.StatusCode)
	}
	if ct := stream.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	buf := make([]byte, 256)
	n, _ := stream.Body.Read(buf)
	cancel()
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: ready") && !strings.Contains(chunk, "data:") {
		t.Fatalf("unexpected stream prelude %q", chunk)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
