package control_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	memstore "motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/runtime/source"
)

// TestModelsE2EDashboardFlow simulates the complete dashboard flow:
// 1. Create MODELS config draft via HTTP API
// 2. Validate draft
// 3. Apply draft
// 4. Read active config revision and verify models field
// 5. Test with wrong max_output_dialect (should fail)
// 6. Verify ActiveModelsConfig reads back correctly
// 7. Verify the JSON response has "models" field (not "payload")
func TestModelsE2EDashboardFlow(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	store := memstore.New()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	api := newControlAPIWithConfig(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	ctx := context.Background()

	// Helper to create a models draft and return the draft_id
	createDraft := func(t *testing.T, modelsPayload map[string]any) string {
		t.Helper()
		resp := mustPOSTJSON(t, server.URL+"/config/drafts", map[string]any{
			"scope":  "MODELS",
			"reason": "e2e audit test",
			"models": modelsPayload,
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("create draft status = %d body=%s", resp.StatusCode, readBody(t, resp))
		}
		var createResp struct {
			Draft domain.ConfigDraft `json:"draft"`
		}
		json.NewDecoder(resp.Body).Decode(&createResp)
		if createResp.Draft.ID == "" {
			t.Fatalf("no draft_id in response")
		}
		return string(createResp.Draft.ID)
	}

	validateAndApply := func(t *testing.T, draftID string) {
		t.Helper()
		clock.Advance(time.Second)
		valResp := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/validate", nil)
		defer valResp.Body.Close()
		if valResp.StatusCode != http.StatusOK {
			t.Fatalf("validate status = %d body=%s", valResp.StatusCode, readBody(t, valResp))
		}
		clock.Advance(time.Second)
		appResp := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/apply", nil)
		defer appResp.Body.Close()
		if appResp.StatusCode != http.StatusOK {
			t.Fatalf("apply status = %d body=%s", appResp.StatusCode, readBody(t, appResp))
		}
	}

	validModelsPayload := func(dialect string) map[string]any {
		return map[string]any{
			"version": "models.v1",
			"providers": []map[string]any{
				{
					"id":                 "groq",
					"kind":               "openai_compatible",
					"base_url":           "https://api.groq.com/openai/v1",
					"api_key_env":        "GROQ_API_KEY",
					"timeout":            int64(90 * time.Second),
					"max_response_bytes": int64(10 * 1024 * 1024),
					"global_limit": map[string]any{
						"resource":       "model-provider:groq",
						"max_concurrent": 2,
						"cooldown_base":  int64(30 * time.Second),
						"cooldown_max":   int64(5 * time.Minute),
					},
				},
			},
			"bindings": []map[string]any{
				{
					"id":                 "binding_groq_llama70b",
					"provider_ref":       "groq",
					"model_id":           "llama-3.3-70b-versatile",
					"enabled":            true,
					"priority":           10,
					"context_tokens":     131072,
					"max_output_tokens":  8192,
					"max_output_dialect": dialect,
					"limit": map[string]any{
						"resource":       "model-binding:binding_groq_llama70b",
						"max_concurrent": 1,
						"cooldown_base":  int64(30 * time.Second),
						"cooldown_max":   int64(5 * time.Minute),
					},
				},
			},
		}
	}

	t.Run("CreateDraft_CorrectDialect_MaxTokens", func(t *testing.T) {
		draftID := createDraft(t, validModelsPayload("max_tokens"))
		t.Logf("created draft: %s", draftID)
		validateAndApply(t, draftID)
		t.Logf("draft validated and applied")

		// Read active config
		activeResp := mustGET(t, server.URL+"/config/revisions/active?scope=MODELS")
		defer activeResp.Body.Close()
		if activeResp.StatusCode != http.StatusOK {
			t.Fatalf("active revision status = %d", activeResp.StatusCode)
		}

		var activeBody struct {
			Revision domain.ConfigRevision `json:"revision"`
		}
		json.NewDecoder(activeResp.Body).Decode(&activeBody)
		if activeBody.Revision.Models == nil {
			t.Fatalf("active revision has no models payload")
		}
		if len(activeBody.Revision.Models.Providers) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(activeBody.Revision.Models.Providers))
		}
		if activeBody.Revision.Models.Providers[0].ID != "groq" {
			t.Fatalf("expected provider groq, got %s", activeBody.Revision.Models.Providers[0].ID)
		}
		if len(activeBody.Revision.Models.Bindings) != 1 {
			t.Fatalf("expected 1 binding, got %d", len(activeBody.Revision.Models.Bindings))
		}
		if activeBody.Revision.Models.Bindings[0].MaxOutputDialect != domain.MaxOutputDialectLegacy {
			t.Fatalf("expected dialect %s, got %s", domain.MaxOutputDialectLegacy, activeBody.Revision.Models.Bindings[0].MaxOutputDialect)
		}
		t.Logf("active models config verified: provider=%s binding=%s dialect=%s",
			activeBody.Revision.Models.Providers[0].ID,
			activeBody.Revision.Models.Bindings[0].ID,
			activeBody.Revision.Models.Bindings[0].MaxOutputDialect)

		// Verify applicability is ConfigNextCycle (hot-reload, not restart)
		if activeBody.Revision.Applicability != domain.ConfigNextCycle {
			t.Fatalf("expected applicability %s, got %s", domain.ConfigNextCycle, activeBody.Revision.Applicability)
		}
		t.Logf("applicability verified: %s", activeBody.Revision.Applicability)

		// Verify ActiveModelsConfig reads it back
		models, found, err := kernel.ActiveModelsConfig(ctx, store)
		if err != nil || !found {
			t.Fatalf("ActiveModelsConfig: found=%v err=%v", found, err)
		}
		if models.Version != "models.v1" {
			t.Fatalf("expected version models.v1, got %s", models.Version)
		}
		t.Logf("ActiveModelsConfig verified: version=%s providers=%d bindings=%d",
			models.Version, len(models.Providers), len(models.Bindings))
	})

	t.Run("CreateDraft_CorrectDialect_MaxCompletionTokens", func(t *testing.T) {
		// Reset store for this subtest
		s2 := memstore.New()
		c2 := source.NewManualClock(time.Date(2026, 8, 5, 13, 0, 0, 0, time.UTC))
		i2 := source.NewSequenceIDGenerator(2)
		a2 := newControlAPIWithConfig(t, s2, c2, i2)
		srv2 := httptest.NewServer(a2.Handler())
		t.Cleanup(srv2.Close)

		resp := mustPOSTJSON(t, srv2.URL+"/config/drafts", map[string]any{
			"scope":  "MODELS",
			"reason": "e2e audit test completion tokens",
			"models": validModelsPayload("max_completion_tokens"),
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("create draft status = %d body=%s", resp.StatusCode, readBody(t, resp))
		}
		var createResp struct {
			Draft domain.ConfigDraft `json:"draft"`
		}
		json.NewDecoder(resp.Body).Decode(&createResp)
		draftID := string(createResp.Draft.ID)

		c2.Advance(time.Second)
		valResp := mustPOSTJSON(t, srv2.URL+"/config/drafts/"+draftID+"/validate", nil)
		defer valResp.Body.Close()
		if valResp.StatusCode != http.StatusOK {
			t.Fatalf("validate status = %d body=%s", valResp.StatusCode, readBody(t, valResp))
		}
		c2.Advance(time.Second)
		appResp := mustPOSTJSON(t, srv2.URL+"/config/drafts/"+draftID+"/apply", nil)
		defer appResp.Body.Close()
		if appResp.StatusCode != http.StatusOK {
			t.Fatalf("apply status = %d body=%s", appResp.StatusCode, readBody(t, appResp))
		}

		// Verify dialect stored correctly
		models, found, err := kernel.ActiveModelsConfig(ctx, s2)
		if err != nil || !found {
			t.Fatalf("ActiveModelsConfig: found=%v err=%v", found, err)
		}
		if models.Bindings[0].MaxOutputDialect != domain.MaxOutputDialectCompletion {
			t.Fatalf("expected dialect %s, got %s", domain.MaxOutputDialectCompletion, models.Bindings[0].MaxOutputDialect)
		}
		t.Logf("completion tokens dialect verified: %s", models.Bindings[0].MaxOutputDialect)
	})

	t.Run("CreateDraft_WrongDialect_Legacy_Fails", func(t *testing.T) {
		// "legacy" is NOT a valid MaxOutputDialect value — must be rejected
		resp := mustPOSTJSON(t, server.URL+"/config/drafts", map[string]any{
			"scope":  "MODELS",
			"reason": "test wrong dialect",
			"models": validModelsPayload("legacy"),
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for wrong dialect 'legacy', got %d body=%s", resp.StatusCode, readBody(t, resp))
		}
		t.Logf("correctly rejected draft with 'legacy' dialect")
	})

	t.Run("ReadActiveConfig_ParsesModelsField_NotPayload", func(t *testing.T) {
		resp := mustGET(t, server.URL+"/config/revisions/active?scope=MODELS")
		defer resp.Body.Close()

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		revision, ok := body["revision"].(map[string]any)
		if !ok {
			t.Fatalf("no revision in response")
		}
		models, ok := revision["models"].(map[string]any)
		if !ok {
			t.Fatalf("no models field in revision (field names: %v)", func() []string {
				keys := make([]string, 0)
				for k := range revision {
					keys = append(keys, k)
				}
				return keys
			}())
		}
		providers, ok := models["providers"].([]any)
		if !ok || len(providers) != 1 {
			t.Fatalf("expected 1 provider in models, got %v", models["providers"])
		}
		t.Logf("models field correctly accessible: providers=%d", len(providers))

		// Verify the JSON field is "models" not "payload"
		if _, exists := revision["payload"]; exists {
			t.Fatalf("revision should NOT have 'payload' field, only 'models'")
		}
		t.Logf("confirmed: revision has 'models' field, not 'payload'")
	})
}

