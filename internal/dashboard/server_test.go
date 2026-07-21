package dashboard_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
		"<option value=\"MODELS\">MODELS</option>",
		"Models / resources / context pressure",
		"/model-bindings",
		"btnModelBindingsList",
		"/model-context-pressures",
		"btnContextPressureList",
		"model-provider:groq",
		"replace-with-operator-confirmed-model-id",
		"body.models = payload",
		"/model-presets",
		"btnPresetRefresh",
		"btnPresetDraft",
		"draft desabilitado criado do preset",
		"Inspetor de execução",
		"btnInspLoad",
		"/operations/",
		"raw_model_outputs",
		"events_truncated",
		"audit_events",
		"projeção de eventos incompleta",
		"GET /events paginado",
		"inspectorRequestGeneration",
		"requestGeneration !== inspectorRequestGeneration",
		"function validStreamCursor(sequence)",
		"function resetStreamCursor(sequence)",
		"function advanceStreamCursor(sequence)",
		"function streamIsCurrent(connectionGeneration)",
		"const connectionGeneration = ++streamGeneration",
		"if (!streamIsCurrent(connectionGeneration)) return",
		"resetStreamCursor(ev.lastEventId)",
		"maxUint64Decimal = \"18446744073709551615\"",
		"/^(0|[1-9][0-9]*)$/.test(next)",
		"next.length > maxUint64Decimal.length",
		"next.length < lastSeq.length",
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

func TestDashboardStreamRejectsInvalidManualCursorBeforeReplacingConnection(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "10x"},
  eventKind: {value: ""},
  timeline: {textContent: "existing stream", dataset: {empty: "0"}},
  streamBadge: {textContent: "SSE live", className: "badge live"},
  globalError: {textContent: ""}
};
const el = (id) => elements[id];
const setError = (msg) => { elements.globalError.textContent = msg; };
const inspectBase = "/api/inspect";
let streamGeneration = 7;
let closeCount = 0;
let es = {close() { closeCount++; }};
class EventSource { constructor() { throw new Error("invalid cursor created EventSource"); } }
` + valid + "\n" + connect + `
connectStream();
if (closeCount !== 0) throw new Error("invalid cursor closed the healthy connection");
if (streamGeneration !== 7) throw new Error("invalid cursor advanced stream generation");
if (!elements.globalError.textContent.includes("uint64")) throw new Error("invalid cursor was not explained");
if (elements.timeline.textContent !== "existing stream") throw new Error("invalid cursor replaced timeline");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard invalid stream cursor behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardMalformedEventStillAdvancesAcceptedCursor(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "10"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""},
  globalError: {textContent: ""}
};
const el = (id) => elements[id];
const setError = (msg) => { elements.globalError.textContent = msg; };
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "10";
class EventSource {
  constructor(url) { this.url = url; this.listeners = {}; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() {}
  emit(kind, event) { this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + connect + `
connectStream();
es.emit("ready", {lastEventId: "10", data: "ready"});
es.emit("event", {lastEventId: "11", data: "{malformed"});
if (lastSeq !== "11" || elements.afterSeq.value !== "11") throw new Error("malformed payload lost accepted SSE cursor");
if (!elements.timeline.textContent.includes("# malformed event")) throw new Error("malformed payload was not labeled");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard malformed stream event behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardStreamGenerationRejectsLateFramesFromClosedConnection(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "900"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => {
  if (!elements[id]) throw new Error("unexpected element " + id);
  return elements[id];
};
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "900";
const streams = [];
class EventSource {
  constructor(url) { this.url = url; this.listeners = {}; this.closed = false; streams.push(this); }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) { if (this.listeners[kind]) this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + connect + `
connectStream();
const first = streams[0];
elements.afterSeq.value = "10";
connectStream();
const second = streams[1];
if (!first.closed) throw new Error("first EventSource was not closed");
second.emit("ready", {lastEventId: "10", data: "second ready"});
second.emit("page", {lastEventId: "20", data: "second page"});
const baseline = JSON.stringify({
  lastSeq,
  afterSeq: elements.afterSeq.value,
  timeline: elements.timeline.textContent,
  badge: elements.streamBadge.textContent,
  badgeClass: elements.streamBadge.className
});
first.emit("ready", {lastEventId: "900", data: "stale ready"});
first.emit("event", {lastEventId: "901", data: JSON.stringify({sequence: 901, kind: "stale"})});
first.emit("page", {lastEventId: "902", data: "stale page"});
first.emit("error", {data: "stale error"});
first.onerror();
const afterStaleCallbacks = JSON.stringify({
  lastSeq,
  afterSeq: elements.afterSeq.value,
  timeline: elements.timeline.textContent,
  badge: elements.streamBadge.textContent,
  badgeClass: elements.streamBadge.className
});
if (afterStaleCallbacks !== baseline) throw new Error("stale callbacks mutated replacement stream state");
if (lastSeq !== "20" || elements.afterSeq.value !== "20") throw new Error("replacement cursor was not authoritative");
if (elements.streamBadge.textContent !== "SSE live") throw new Error("stale onerror changed replacement badge");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard stream generation behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardStreamCursorReadyResetsNewStreamBaseline(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	script := `
const maxUint64Decimal = "18446744073709551615";
const afterSeq = {value: "0"};
const el = (id) => { if (id !== "afterSeq") throw new Error("unexpected element " + id); return afterSeq; };
let lastSeq = "0";
` + valid + "\n" + reset + "\n" + advance + `
resetStreamCursor("900");
resetStreamCursor("10");
if (lastSeq !== "10" || afterSeq.value !== "10") throw new Error("ready did not reset baseline");
advanceStreamCursor("250");
if (lastSeq !== "250" || afterSeq.value !== "250") throw new Error("page did not advance from reset baseline");
advanceStreamCursor("200");
if (lastSeq !== "250" || afterSeq.value !== "250") throw new Error("regressive frame was accepted");
resetStreamCursor("18446744073709551616");
if (lastSeq !== "250") throw new Error("uint64 overflow was accepted");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard cursor behavior failed: %v\n%s", err, output)
	}
}

func renderDashboardForTest(t *testing.T) string {
	t.Helper()
	ui, err := dashboard.New(http.NotFoundHandler(), http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ui.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", recorder.Code)
	}
	return recorder.Body.String()
}

func extractJSFunction(t *testing.T, source, name string) string {
	t.Helper()
	start := strings.Index(source, "function "+name+"(")
	if start < 0 {
		t.Fatalf("JavaScript function %s not found", name)
	}
	open := strings.Index(source[start:], "{")
	if open < 0 {
		t.Fatalf("JavaScript function %s has no body", name)
	}
	open += start
	depth := 0
	for i := open; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : i+1]
			}
		}
	}
	t.Fatalf("JavaScript function %s is unterminated", name)
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
