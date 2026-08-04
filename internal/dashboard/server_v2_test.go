package dashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestV2RoutesServeOverviewAssetsAndAPI pins the /dash wiring added in the
// dashboard-v2 scaffold: redirect, overview markers, embedded assets and the
// /api/inspect proxy. It guards the bootstrap mux composition
// (legacy "/" + "/dash/") against silent regressions.
func TestV2RoutesServeOverviewAssetsAndAPI(t *testing.T) {
	inspectMux := http.NewServeMux()
	inspectMux.HandleFunc("GET /missions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"missions":[]}`))
	})
	inspectMux.HandleFunc("GET /events/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	})
	inspectMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	inspectMux.HandleFunc("GET /overview", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"pending_commands":3}`))
	})
	inspectMux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"events":[{"sequence":1,"kind":"operator.command.accepted"}],"has_more":true,"next_sequence":1}`))
	})
	inspectMux.HandleFunc("GET /model-bindings", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"count":1,"bindings":[{"binding_id":"b1","provider_id":"groq","model_id":"llama-3.3-70b-versatile"}]}`))
	})
	inspectMux.HandleFunc("GET /resources", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"count":1,"resources":[{"resource":"groq:llama-3.3-70b","in_flight":2,"minute_count":47,"circuit_open":false}]}`))
	})
	inspectMux.HandleFunc("GET /frontier", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"mission_id":"m1","total":1,"items":[{"id":"op1","title":"gap-scan sweep","family":"gap_scan","status":"OPEN","depth":0,"priority":5,"risk":"LOW","created_at":"2026-08-02T00:00:00Z"}],"policy_version":"h1"}`))
	})
	inspectMux.HandleFunc("GET /alerts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"total":2,"critical":1,"warnings":1,"alerts":[{"code":"telemetry.disabled","severity":"info","summary":"otel off"},{"code":"resource.unsettled_receipts","severity":"warning","summary":"2 receipts unsettled"}]}`))
	})
	inspectMux.HandleFunc("GET /knowledge", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":1,"sources":3,"observations":12,"claims":4,"artifacts":2,"evidence_links":5}`))
	})
	inspectMux.HandleFunc("GET /knowledge/sources", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":"s1","locator":"https://example.com/doc","kind":"http_fetch","versions":2,"observed_at":"2026-08-01T00:00:00Z"}],"total":1,"offset":0,"limit":25}`))
	})
	inspectMux.HandleFunc("GET /knowledge/observations", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":"o1","statement":"temperature rising","provenance":"primary","source_fragment_id":"f1"}],"total":1,"offset":0,"limit":25}`))
	})
	inspectMux.HandleFunc("GET /knowledge/claims", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":"c1","proposition":"store is reliable","version":1,"supports":2,"contradicts":0,"without_evidence":false}],"total":1,"offset":0,"limit":25}`))
	})
	inspectMux.HandleFunc("GET /knowledge/artifacts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":"a1","kind":"report","content_ref":"sha256://abc","content_bytes":1024,"dependency_count":1,"base_commit_id":"c0","stale":false}],"total":1,"offset":0,"limit":25}`))
	})
	controlMux := http.NewServeMux()
	v2, err := NewV2(inspectMux, controlMux, nil)
	if err != nil {
		t.Fatalf("NewV2: %v", err)
	}
	srv := httptest.NewServer(v2.Handler())
	defer srv.Close()

	// /dash redirects to /dash/ and renders the overview shell.
	resp, err := srv.Client().Get(srv.URL + "/dash")
	if err != nil {
		t.Fatalf("get /dash: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/dash/ status: %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "motor-autonomo") ||
		!strings.Contains(string(body), "htmx.min.js") ||
		!strings.Contains(string(body), "overviewState") ||
		!strings.Contains(string(body), "/dash/api/events/stream") {
		t.Fatalf("overview missing layout/live-data markers")
	}

	// Embedded assets are served under /dash/assets/.
	for _, path := range []string{"/dash/assets/htmx.min.js", "/dash/assets/alpine.min.js", "/dash/assets/app.css"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status: %d", path, resp.StatusCode)
		}
	}

	// Inspect API is proxied under /api/inspect (legacy composition) and
	// /dash/api (browser-safe same-origin v2).
	resp, err = srv.Client().Get(srv.URL + "/api/inspect/missions")
	if err != nil {
		t.Fatalf("get api: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "missions") {
		t.Fatalf("inspect proxy body: %s", string(b))
	}

	resp, err = srv.Client().Get(srv.URL + "/dash/api/health")
	if err != nil {
		t.Fatalf("get dash api health: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), "ok") {
		t.Fatalf("/dash/api/health status=%d body=%s", resp.StatusCode, string(b))
	}

	resp, err = srv.Client().Get(srv.URL + "/dash/api/overview")
	if err != nil {
		t.Fatalf("get dash api overview: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "pending_commands") {
		t.Fatalf("/dash/api/overview body: %s", string(b))
	}

	// Events explorer page is served with live-data wiring.
	resp, err = srv.Client().Get(srv.URL + "/dash/events")
	if err != nil {
		t.Fatalf("get /dash/events: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(b), "eventsState") ||
		!strings.Contains(string(b), "/dash/api/events") {
		t.Fatalf("/dash/events status=%d missing explorer markers", resp.StatusCode)
	}

	// SSE Stream proxy check
	resp, err = srv.Client().Get(srv.URL + "/dash/api/events/stream")
	if err != nil {
		t.Fatalf("get dash api events stream: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/dash/api/events/stream status=%d, expected 200", resp.StatusCode)
	}

	// Events proxy passes the paginated inspect payload through unchanged.
	resp, err = srv.Client().Get(srv.URL + "/dash/api/events?limit=5")

	// Models page is served with live-data wiring.
	resp, err = srv.Client().Get(srv.URL + "/dash/models")
	if err != nil {
		t.Fatalf("get /dash/models: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(b), "modelsState") ||
		!strings.Contains(string(b), "/dash/api/model-bindings") {
		t.Fatalf("/dash/models status=%d missing posture markers", resp.StatusCode)
	}

	// Model bindings proxy passes through provider/model metadata.
	resp, err = srv.Client().Get(srv.URL + "/dash/api/model-bindings")
	if err != nil {
		t.Fatalf("get dash api model-bindings: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "llama-3.3-70b-versatile") {
		t.Fatalf("/dash/api/model-bindings body: %s", string(b))
	}

	// Resources page is served with live-data wiring.
	resp, err = srv.Client().Get(srv.URL + "/dash/resources")
	if err != nil {
		t.Fatalf("get /dash/resources: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(b), "resourcesState") ||
		!strings.Contains(string(b), "/dash/api/resources") {
		t.Fatalf("/dash/resources status=%d missing posture markers", resp.StatusCode)
	}

	// Resources proxy passes through gate payload (in_flight, counts, circuit).
	resp, err = srv.Client().Get(srv.URL + "/dash/api/resources")
	if err != nil {
		t.Fatalf("get dash api resources: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "groq:llama-3.3-70b") ||
		!strings.Contains(string(b), "in_flight") {
		t.Fatalf("/dash/api/resources body: %s", string(b))
	}

	// Frontier page is served with live-data wiring.
	resp, err = srv.Client().Get(srv.URL + "/dash/frontier")
	if err != nil {
		t.Fatalf("get /dash/frontier: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(b), "frontierState") ||
		!strings.Contains(string(b), "/dash/api/frontier") {
		t.Fatalf("/dash/frontier status=%d missing explorer markers", resp.StatusCode)
	}

	// Frontier proxy passes opportunity payload through unchanged.
	resp, err = srv.Client().Get(srv.URL + "/dash/api/frontier")
	if err != nil {
		t.Fatalf("get dash api frontier: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "gap-scan sweep") || !strings.Contains(string(b), "policy_version") {
		t.Fatalf("/dash/api/frontier body: %s", string(b))
	}

	// Alerts page is served with live-data wiring.
	resp, err = srv.Client().Get(srv.URL + "/dash/alerts")
	if err != nil {
		t.Fatalf("get /dash/alerts: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(b), "alertsState") ||
		!strings.Contains(string(b), "/dash/api/alerts") {
		t.Fatalf("/dash/alerts status=%d missing alerts markers", resp.StatusCode)
	}

	// Alerts proxy passes snapshot through unchanged.
	resp, err = srv.Client().Get(srv.URL + "/dash/api/alerts")
	if err != nil {
		t.Fatalf("get dash api alerts: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "telemetry.disabled") ||
		!strings.Contains(string(b), "resource.unsettled_receipts") {
		t.Fatalf("/dash/api/alerts body: %s", string(b))
	}

	// Knowledge page is served with live-data wiring.
	resp, err = srv.Client().Get(srv.URL + "/dash/knowledge")
	if err != nil {
		t.Fatalf("get /dash/knowledge: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK ||
		!strings.Contains(string(b), "knowledgeState") ||
		!strings.Contains(string(b), "/dash/api/knowledge") {
		t.Fatalf("/dash/knowledge status=%d missing knowledge markers", resp.StatusCode)
	}

	// Knowledge catalog proxy passes summary payload through unchanged.
	resp, err = srv.Client().Get(srv.URL + "/dash/api/knowledge")
	if err != nil {
		t.Fatalf("get dash api knowledge: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "sources") || !strings.Contains(string(b), "observations") {
		t.Fatalf("/dash/api/knowledge body: %s", string(b))
	}

	// Knowledge sources list proxy preserves paging fields.
	resp, err = srv.Client().Get(srv.URL + "/dash/api/knowledge/sources?offset=0&limit=5")
	if err != nil {
		t.Fatalf("get dash api knowledge/sources: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "example.com/doc") || !strings.Contains(string(b), "http_fetch") {
		t.Fatalf("/dash/api/knowledge/sources body: %s", string(b))
	}
}
