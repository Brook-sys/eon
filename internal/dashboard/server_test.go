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
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
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
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + connect + `
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

func TestDashboardStreamConstructionFailurePreservesHealthyConnection(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "10"},
  eventKind: {value: ""},
  timeline: {textContent: "healthy timeline", dataset: {empty: "0"}},
  streamBadge: {textContent: "SSE live", className: "badge live"},
  globalError: {textContent: ""}
};
const el = (id) => elements[id];
const setError = (msg) => { elements.globalError.textContent = msg; };
const inspectBase = "/api/inspect";
let streamGeneration = 4;
let closeCount = 0;
const healthy = {close() { closeCount++; }};
let es = healthy;
class EventSource { constructor() { throw new Error("constructor failed"); } }
` + valid + "\n" + connect + `
connectStream();
if (es !== healthy || closeCount !== 0) throw new Error("failed candidate replaced healthy connection");
if (streamGeneration !== 4) throw new Error("failed candidate advanced stream generation");
if (elements.timeline.textContent !== "healthy timeline") throw new Error("failed candidate replaced timeline");
if (elements.streamBadge.textContent !== "SSE live") throw new Error("failed candidate replaced badge");
if (!elements.globalError.textContent.includes("constructor failed")) throw new Error("construction failure was not explained");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard stream construction failure behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardInvalidFrameCursorFailsClosed(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
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
  constructor() { this.listeners = {}; this.closed = false; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) { if (this.listeners[kind]) this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + connect + `
connectStream();
const stream = es;
stream.emit("ready", {lastEventId: "10", data: "ready"});
stream.emit("page", {lastEventId: "9", data: "regressive page"});
if (!stream.closed || es !== null) throw new Error("invalid frame did not close the stream");
if (streamGeneration !== 2) throw new Error("invalid frame did not invalidate callbacks");
if (lastSeq !== "10" || elements.afterSeq.value !== "10") throw new Error("invalid frame mutated durable cursor");
if (elements.streamBadge.textContent !== "SSE protocol error") throw new Error("protocol failure was not visible");
if (!elements.timeline.textContent.includes("# protocol error")) throw new Error("protocol failure was not recorded");
const baseline = elements.timeline.textContent;
stream.emit("event", {lastEventId: "11", data: JSON.stringify({sequence: 11, sequence_decimal: "11"})});
if (elements.timeline.textContent !== baseline || lastSeq !== "10") throw new Error("callback after protocol failure was not fenced");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard invalid frame cursor behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardServerErrorFrameStopsAutomaticReconnect(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
	failServer := extractJSFunction(t, html, "failStreamServer")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "10"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => elements[id];
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "10";
class EventSource {
  constructor() { this.listeners = {}; this.closed = false; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) { if (this.listeners[kind]) this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + failServer + "\n" + connect + `
connectStream();
const stream = es;
stream.emit("ready", {lastEventId: "10", data: "ready"});
stream.emit("terminal_error", {data: JSON.stringify({code: "stream_list_failed"})});
if (!stream.closed || es !== null) throw new Error("terminal server error did not close and clear stream");
if (streamGeneration !== 2) throw new Error("terminal server error did not fence queued callbacks");
if (lastSeq !== "10" || elements.afterSeq.value !== "10") throw new Error("terminal server error mutated cursor");
if (elements.streamBadge.textContent !== "SSE server error") throw new Error("terminal server failure was not distinguished");
if (!elements.timeline.textContent.includes("stream_list_failed")) throw new Error("terminal server failure was not recorded");
const baseline = JSON.stringify(elements);
stream.onerror();
if (JSON.stringify(elements) !== baseline) throw new Error("queued native onerror overwrote terminal failure");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard terminal server error behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardNativeStreamErrorKeepsAutomaticReconnect(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
	failServer := extractJSFunction(t, html, "failStreamServer")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "10"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => elements[id];
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "10";
class EventSource {
  constructor() { this.listeners = {}; this.closed = false; this.onerror = null; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) {
    if (this.listeners[kind]) this.listeners[kind](event);
    if (kind === "error" && this.onerror) this.onerror(event);
  }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + failServer + "\n" + connect + `
connectStream();
const stream = es;
stream.emit("ready", {lastEventId: "10", data: "ready"});
const generationBeforeError = streamGeneration;
stream.emit("error", {});
if (stream.closed) throw new Error("native transport error closed reconnectable stream");
if (es !== stream) throw new Error("native transport error cleared current stream");
if (streamGeneration !== generationBeforeError) throw new Error("native transport error fenced its own reconnect callbacks");
if (lastSeq !== "10" || elements.afterSeq.value !== "10") throw new Error("native transport error mutated cursor");
if (elements.streamBadge.textContent !== "SSE error/retry") throw new Error("native transport retry was not visible");
stream.emit("ready", {lastEventId: "10", data: "reconnected"});
stream.emit("event", {lastEventId: "11", data: JSON.stringify({sequence: 11, sequence_decimal: "11", kind: "continued"})});
if (stream.closed || es !== stream) throw new Error("reconnected stream was not retained");
if (streamGeneration !== generationBeforeError) throw new Error("accepted reconnect changed connection generation");
if (lastSeq !== "11" || elements.afterSeq.value !== "11") throw new Error("callbacks after reconnect were fenced");
if (elements.streamBadge.textContent !== "SSE live") throw new Error("ready after native retry did not restore live badge");
if (!elements.timeline.textContent.includes("continued")) throw new Error("event after native retry was not rendered");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard native stream retry behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardReconnectReadyCannotRewindAcceptedCursor(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
	failServer := extractJSFunction(t, html, "failStreamServer")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "900"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => elements[id];
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "900";
class EventSource {
  constructor() { this.listeners = {}; this.closed = false; this.onerror = null; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) { if (this.listeners[kind]) this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + failServer + "\n" + connect + `
connectStream();
const stream = es;
stream.emit("ready", {lastEventId: "900", data: "initial"});
stream.emit("event", {lastEventId: "950", data: JSON.stringify({sequence: 950, sequence_decimal: "950", kind: "accepted"})});
stream.onerror();
stream.emit("ready", {lastEventId: "900", data: "reconnect stale baseline"});
if (!stream.closed || es !== null) throw new Error("regressive reconnect ready did not close the stream");
if (streamGeneration !== 2) throw new Error("regressive reconnect ready did not fence callbacks");
if (lastSeq !== "950" || elements.afterSeq.value !== "950") throw new Error("regressive reconnect ready rewound cursor");
if (elements.streamBadge.textContent !== "SSE protocol error") throw new Error("regressive reconnect ready was not visible");
const baseline = elements.timeline.textContent;
stream.emit("event", {lastEventId: "951", data: JSON.stringify({sequence: 951, sequence_decimal: "951", kind: "stale"})});
if (elements.timeline.textContent !== baseline || lastSeq !== "950") throw new Error("callback after reconnect protocol failure was not fenced");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard reconnect ready cursor behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardRepeatedEventCursorFailsClosedWithoutRenderingReplay(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
	failServer := extractJSFunction(t, html, "failStreamServer")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "40"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => elements[id];
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "40";
class EventSource {
  constructor() { this.listeners = {}; this.closed = false; this.onerror = null; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) { if (this.listeners[kind]) this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + failServer + "\n" + connect + `
connectStream();
const stream = es;
stream.emit("ready", {lastEventId: "40", data: "initial"});
stream.emit("event", {lastEventId: "41", data: JSON.stringify({sequence: 41, sequence_decimal: "41", kind: "accepted"})});
const acceptedTimeline = elements.timeline.textContent;
stream.emit("event", {lastEventId: "41", data: JSON.stringify({sequence: 41, sequence_decimal: "41", kind: "replayed-conflict"})});
if (!stream.closed || es !== null) throw new Error("repeated event cursor did not close the stream");
if (streamGeneration !== 2) throw new Error("repeated event cursor did not fence callbacks");
if (lastSeq !== "41" || elements.afterSeq.value !== "41") throw new Error("repeated event cursor changed accepted cursor");
if (elements.streamBadge.textContent !== "SSE protocol error") throw new Error("repeated event cursor was not visible as protocol error");
if (elements.timeline.textContent.includes("replayed-conflict")) throw new Error("replayed payload was rendered as fresh evidence");
if (!elements.timeline.textContent.startsWith(acceptedTimeline)) throw new Error("accepted timeline was not preserved");
const baseline = elements.timeline.textContent;
stream.emit("event", {lastEventId: "42", data: JSON.stringify({sequence: 42, sequence_decimal: "42", kind: "stale"})});
if (elements.timeline.textContent !== baseline || lastSeq !== "41") throw new Error("callback after repeated event failure was not fenced");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard repeated event cursor behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardEventPayloadSequenceMustMatchAcceptedCursor(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
	failServer := extractJSFunction(t, html, "failStreamServer")
	connect := extractJSFunction(t, html, "connectStream")
	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "9007199254740992"},
  eventKind: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => elements[id];
const inspectBase = "/api/inspect";
let es = null;
let streamGeneration = 0;
let lastSeq = "9007199254740992";
class EventSource {
  constructor() { this.listeners = {}; this.closed = false; }
  addEventListener(kind, callback) { this.listeners[kind] = callback; }
  close() { this.closed = true; }
  emit(kind, event) { if (this.listeners[kind]) this.listeners[kind](event); }
}
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + failServer + "\n" + connect + `
connectStream();
const stream = es;
stream.emit("ready", {lastEventId: "9007199254740992", data: "ready"});
stream.emit("event", {lastEventId: "9007199254740993", data: JSON.stringify({sequence: 9007199254740993, sequence_decimal: "9007199254740992", kind: "mismatch"})});
if (!stream.closed || es !== null) throw new Error("mismatched payload sequence did not close stream");
if (lastSeq !== "9007199254740993") throw new Error("accepted SSE cursor was not preserved exactly");
if (elements.timeline.textContent.includes("mismatch")) throw new Error("mismatched payload was rendered as evidence");
if (!elements.timeline.textContent.includes("sequence_decimal")) throw new Error("protocol mismatch was not explained");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard event payload sequence behavior failed: %v\n%s", err, output)
	}
}

func TestDashboardTimelineRetentionIsBoundedAndVisible(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	appendLine := extractJSFunction(t, html, "appendTimeline")
	script := `
const timeline = {textContent: "aguardando eventos…", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0};
const el = (id) => { if (id !== "timeline") throw new Error("unexpected element " + id); return timeline; };
const bytes = (value) => Buffer.byteLength(value, "utf8");
` + appendLine + `
for (let i = 0; i < 2000; i++) appendTimeline("event-" + String(i).padStart(4, "0") + " " + "😀".repeat(80));
const retained = timeline.textContent.trimEnd().split("\n");
if (retained.length > 400) throw new Error("timeline retained too many lines: " + retained.length);
if (bytes(timeline.textContent) > 65536) throw new Error("timeline retained too many UTF-8 bytes: " + bytes(timeline.textContent));
if (retained[0] !== "# older timeline entries omitted") throw new Error("timeline omission is not visible");
if (!timeline.textContent.includes("event-1999")) throw new Error("newest timeline entry was not retained");
if (timeline.textContent.includes("event-0000")) throw new Error("oldest timeline entry was not evicted");
if (timeline.textContent.includes("�")) throw new Error("UTF-8 retention split a code point");
const boundedBytes = bytes(timeline.textContent);
for (let i = 2000; i < 4000; i++) appendTimeline("event-" + String(i).padStart(4, "0") + " " + "界".repeat(80));
if (bytes(timeline.textContent) > 65536) throw new Error("continued appends exceeded UTF-8 byte bound");
if (timeline.textContent.trimEnd().split("\n").length > 400) throw new Error("continued appends exceeded line bound");
if (!timeline.textContent.includes("event-3999")) throw new Error("continued appends lost newest entry");
if (bytes(timeline.textContent) > boundedBytes + 400) throw new Error("retention kept growing after reaching its bound");
if (timeline.textContent.includes("�")) throw new Error("continued UTF-8 retention split a code point");
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard bounded timeline behavior failed: %v\n%s", err, output)
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
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
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
` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + connect + `
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
first.emit("event", {lastEventId: "901", data: JSON.stringify({sequence: 901, sequence_decimal: "901", kind: "stale"})});
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
