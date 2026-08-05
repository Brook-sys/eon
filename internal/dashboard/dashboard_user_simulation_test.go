package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/runtime/bootstrap"
)

// TestDashboardUserSimulations multi-scenario integration suite
// simulating real user interaction with the dashboard and Control/Inspect APIs.
func TestDashboardUserSimulations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "eon-dash-e2e-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "sim.db")
	opts := bootstrap.Options{
		ListenAddr:      "127.0.0.1:0",
		StoreBackend:    bootstrap.StorageSQLite,
		SQLitePath:      dbPath,
		MissionID:       "m-dash-user-sim",
		EnableDashboard: true,
		IdleMin:         10 * time.Millisecond,
		IdleMax:         50 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatalf("bootstrap.Open: %v", err)
	}
	defer rt.Close(ctx)

	go func() {
		_ = rt.RunControlLoop(ctx)
	}()

	srv := httptest.NewServer(rt.Handler)
	defer srv.Close()

	client := srv.Client()

	t.Run("Scenario 1: Page Navigation and Asset Verification", func(t *testing.T) {
		pages := []string{
			"/dash/",
			"/dash/models",
			"/dash/resources",
			"/dash/frontier",
			"/dash/alerts",
			"/dash/knowledge",
			"/dash/events",
		}
		for _, page := range pages {
			resp, err := client.Get(srv.URL + page)
			if err != nil {
				t.Fatalf("GET %s failed: %v", page, err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s returned HTTP %d, body: %s", page, resp.StatusCode, string(body))
			}
		}

		assets := []string{
			"/dash/assets/htmx.min.js",
			"/dash/assets/alpine.min.js",
			"/dash/assets/app.css",
		}
		for _, asset := range assets {
			resp, err := client.Get(srv.URL + asset)
			if err != nil {
				t.Fatalf("GET %s failed: %v", asset, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET asset %s returned HTTP %d", asset, resp.StatusCode)
			}
		}
	})

	t.Run("Scenario 2: User Mission Ingestion and Execution Tracking", func(t *testing.T) {
		// 1. User submits a mission command via /dash/api/control/commands
		missionCmd := map[string]interface{}{
			"schema_version":    1,
			"idempotency_key":   "dash-user-sim-1",
			"kind":              "PAUSE_MISSION",
			"target":            map[string]interface{}{"mission_id": "m-dash-user-sim"},
			"expected_revision": 1,
			"reason":            "Dash User Sim Pause Test",
		}
		buf, _ := json.Marshal(missionCmd)
		resp, err := client.Post(srv.URL+"/dash/api/control/commands", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("POST /dash/api/control/commands: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			t.Fatalf("Admit mission returned HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Allow control loop to process cycle
		time.Sleep(100 * time.Millisecond)

		// 2. User checks inspect overview via /dash/api/overview
		resp, err = client.Get(srv.URL + "/dash/api/overview")
		if err != nil {
			t.Fatalf("GET /dash/api/overview: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Overview returned HTTP %d: %s", resp.StatusCode, string(body))
		}

		// 3. User inspects mission endpoint
		resp, err = client.Get(srv.URL + "/dash/api/missions/m-dash-user-sim")
		if err != nil {
			t.Fatalf("GET mission: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("Get mission returned HTTP %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("Scenario 3: Models Draft Workflow (Create -> Validate -> Apply)", func(t *testing.T) {
		err = os.Setenv("GROQ_API_KEY", "dummy-key-for-simulation")
		if err != nil {
			t.Fatalf("set env: %v", err)
		}
		defer os.Unsetenv("GROQ_API_KEY")
		// Create draft
		draftReq := map[string]interface{}{
			"schema_version": 1,
			"scope":          "MODELS",
			"reason":         "Adicionando provedor Groq via Dashboard UI",
			"models": map[string]interface{}{
				"version": "models.v1",
				"providers": []map[string]interface{}{
					{
						"id":                 "groq-user-test",
						"kind":               "openai_compatible",
						"base_url":           "https://api.groq.com/openai/v1",
						"api_key_env":        "GROQ_API_KEY",
						"timeout":            90000000000,
						"max_response_bytes": 10485760,
						"global_limit": map[string]interface{}{
							"resource":"model-provider:groq-user-test",
							"max_concurrent": 4,
						},
					},
				},
				"bindings": []map[string]interface{}{
					{
						"id":                 "binding_groq_llama70b",
						"provider_ref":       "groq-user-test",
						"model_id":           "llama-3.3-70b-versatile",
						"enabled":            true,
						"priority":           10,
						"context_tokens":     131072,
						"max_output_tokens":  8192,
						"max_output_dialect": "max_tokens",
						"limit": map[string]interface{}{
							"resource":       "model-binding:binding_groq_llama70b",
							"max_concurrent": 2,
						},
					},
				},
			},
		}
		buf, _ := json.Marshal(draftReq)
		resp, err := client.Post(srv.URL+"/dash/api/control/config/drafts", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("POST create draft: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			t.Fatalf("Create draft failed HTTP %d: %s", resp.StatusCode, string(body))
		}

		var draftRes struct {
			DraftID string `json:"draft_id"`
			Draft   struct {
				DraftID string `json:"draft_id"`
			} `json:"draft"`
		}
		_ = json.Unmarshal(body, &draftRes)
		draftID := draftRes.DraftID
		if draftID == "" {
			draftID = draftRes.Draft.DraftID
		}
		if draftID == "" {
			t.Fatalf("No draft_id returned in response: %s", string(body))
		}

		// Validate draft
		resp, err = client.Post(srv.URL+"/dash/api/control/config/drafts/"+draftID+"/validate", "application/json", nil)
		if err != nil {
			t.Fatalf("POST validate draft: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Validate draft failed HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Apply draft
		resp, err = client.Post(srv.URL+"/dash/api/control/config/drafts/"+draftID+"/apply", "application/json", nil)
		if err != nil {
			t.Fatalf("POST apply draft: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Apply draft failed HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Verify active models revision
		resp, err = client.Get(srv.URL + "/dash/api/control/config/revisions/active?scope=MODELS")
		if err != nil {
			t.Fatalf("GET active revision: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Get active revision failed HTTP %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("Scenario 4: Vault Credential Management Workflow", func(t *testing.T) {
		// Initial status
		resp, err := client.Get(srv.URL + "/dash/api/vault/status")
		if err != nil {
			t.Fatalf("GET vault status: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Vault status returned HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Initialize vault
		initReq := map[string]string{"password": "UserPass123!Secure"}
		buf, _ := json.Marshal(initReq)
		resp, err = client.Post(srv.URL+"/dash/api/vault/initialize", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("POST vault init: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("Vault init returned HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Store secret for provider
		secReq := map[string]string{"value": "gsk_test_key_12345"}
		buf, _ = json.Marshal(secReq)
		secName := "provider/groq-user-test/api-key"
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/dash/api/vault/secrets/"+url.PathEscape(secName), bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("POST secret: %v", err)
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Fatalf("Store secret returned HTTP %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("Scenario 5: Adverse Inputs & Unknown Endpoint Fails Gracefully", func(t *testing.T) {
		// Unknown API route
		resp, err := client.Get(srv.URL + "/dash/api/nonexistent-route")
		if err != nil {
			t.Fatalf("GET invalid route: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected HTTP 404 for unknown route, got %d", resp.StatusCode)
		}

		// Malformed JSON command
		resp, err = client.Post(srv.URL+"/dash/api/control/commands", "application/json", bytes.NewReader([]byte("{invalid-json")))
		if err != nil {
			t.Fatalf("POST malformed json: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("Expected 400/422 for malformed json, got %d", resp.StatusCode)
		}

		// Invalid draft validate (nonexistent draft id)
		resp, err = client.Post(srv.URL+"/dash/api/control/config/drafts/cfgdraft_nonexistent/validate", "application/json", nil)
		if err != nil {
			t.Fatalf("POST validate invalid draft: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected HTTP 404 for invalid draft validate, got %d: %s", resp.StatusCode, string(body))
		}
	})
}
