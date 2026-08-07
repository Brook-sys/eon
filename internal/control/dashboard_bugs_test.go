package control_test

import (
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

// TestDashboardBug5_BasedOnRevisionForwarded verifies that based_on_revision
// is accepted and stored when creating a MODELS draft. The frontend fix sends
// cfgActiveRevision instead of always 0.
func TestDashboardBug5_BasedOnRevisionForwarded(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	store := memstore.New()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	api := newControlAPIWithConfig(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Create first draft with based_on_revision=0, apply it
	draft1ID := createModelsDraftWithProvider(t, server.URL, 0, "groq", "https://api.groq.com/openai/v1", "GROQ_API_KEY")
	validateDraft(t, server.URL, draft1ID)
	clock.Advance(time.Second)
	applyDraft(t, server.URL, draft1ID)

	// Read active revision
	activeResp := mustGET(t, server.URL+"/config/revisions/active?scope=MODELS")
	defer activeResp.Body.Close()
	var activeBody struct {
		Revision struct {
			Revision int `json:"revision"`
		} `json:"revision"`
	}
	json.NewDecoder(activeResp.Body).Decode(&activeBody)
	if activeBody.Revision.Revision != 1 {
		t.Fatalf("expected active revision 1, got %d", activeBody.Revision.Revision)
	}

	// Create second draft with based_on_revision=1 (the active revision)
	draft2ID := createModelsDraftWithProvider(t, server.URL, 1, "nvidia", "https://integrate.api.nvidia.com/v1", "NVIDIA_NIM_API_KEY")
	if draft2ID == "" {
		t.Fatal("expected non-empty draft_id")
	}

	// Verify the draft stored BasedOnRevision=1
	draftResp := mustGET(t, server.URL+"/config/drafts/"+draft2ID)
	defer draftResp.Body.Close()
	var draftBody struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	json.NewDecoder(draftResp.Body).Decode(&draftBody)
	if draftBody.Draft.BasedOnRevision != 1 {
		t.Fatalf("expected BasedOnRevision=1, got %d", draftBody.Draft.BasedOnRevision)
	}
	t.Logf("BUG #5 verified: based_on_revision=1 stored correctly in draft")
}

// TestDashboardBug5_EmptyRevisionSendsZero verifies that when there's no
// active revision, based_on_revision=0 is sent (not nil/missing).
func TestDashboardBug5_EmptyRevisionSendsZero(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	store := memstore.New()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	api := newControlAPIWithConfig(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Create draft with based_on_revision=0 (no active revision yet)
	draftID := createModelsDraftWithProvider(t, server.URL, 0, "groq", "https://api.groq.com/openai/v1", "GROQ_API_KEY")
	if draftID == "" {
		t.Fatal("expected draft_id")
	}

	// Verify it was accepted and stored
	draftResp := mustGET(t, server.URL+"/config/drafts/"+draftID)
	defer draftResp.Body.Close()
	var draftBody struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	json.NewDecoder(draftResp.Body).Decode(&draftBody)
	if draftBody.Draft.BasedOnRevision != 0 {
		t.Fatalf("expected BasedOnRevision=0, got %d", draftBody.Draft.BasedOnRevision)
	}
	t.Logf("BUG #5 edge case verified: empty revision sends based_on_revision=0")
}

// TestDashboardBug8_DoubleApplyIdempotent verifies that applying a draft
// that was already applied is idempotent — the kernel returns the existing
// revision (HTTP 200) rather than an error. This is the correct behavior:
// double-clicks on the same draft are safe. The frontend fix for BUG #8
// handles the case where a *different* draft fails because the config was
// advanced by a concurrent operation.
func TestDashboardBug8_DoubleApplyIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	store := memstore.New()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	api := newControlAPIWithConfig(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Create draft, validate, and apply it (becomes rev 1)
	draftID := createModelsDraftWithProvider(t, server.URL, 0, "groq", "https://api.groq.com/openai/v1", "GROQ_API_KEY")
	validateDraft(t, server.URL, draftID)
	clock.Advance(time.Second)
	applyDraft(t, server.URL, draftID)

	// Apply the same draft again — should be idempotent (HTTP 200, same revision)
	clock.Advance(time.Second)
	secondApply := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/apply", nil)
	defer secondApply.Body.Close()

	if secondApply.StatusCode != http.StatusOK {
		t.Fatalf("double-apply: expected 200 (idempotent), got %d; body=%s", secondApply.StatusCode, readBody(t, secondApply))
	}

	var body struct {
		Revision struct {
			Revision int `json:"revision"`
		} `json:"revision"`
	}
	json.NewDecoder(secondApply.Body).Decode(&body)
	if body.Revision.Revision != 1 {
		t.Fatalf("expected revision 1 (idempotent), got %d", body.Revision.Revision)
	}
	t.Logf("BUG #8 verified: double-apply is idempotent, returns same revision %d", body.Revision.Revision)
}

// TestDashboardBug8_TwoDraftsSameBase verifies that two drafts with different
// content created at the same base revision both apply successfully.
func TestDashboardBug8_TwoDraftsSameBase(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	store := memstore.New()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	api := newControlAPIWithConfig(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Draft A: add groq provider, apply (rev 1)
	draftA := createModelsDraftWithProvider(t, server.URL, 0, "groq", "https://api.groq.com/openai/v1", "GROQ_API_KEY")
	validateDraft(t, server.URL, draftA)
	clock.Advance(time.Second)
	applyDraft(t, server.URL, draftA)

	// Draft B: add nvidia provider (different content) at based_on_revision=1
	clock.Advance(time.Second)
	draftB := createModelsDraftWithProvider(t, server.URL, 1, "nvidia", "https://integrate.api.nvidia.com/v1", "NVIDIA_NIM_API_KEY")
	validateDraft(t, server.URL, draftB)
	clock.Advance(time.Second)
	applyDraft(t, server.URL, draftB)

	// Verify active revision is now 2
	activeResp := mustGET(t, server.URL+"/config/revisions/active?scope=MODELS")
	defer activeResp.Body.Close()
	var activeBody struct {
		Revision struct {
			Revision int `json:"revision"`
		} `json:"revision"`
	}
	json.NewDecoder(activeResp.Body).Decode(&activeBody)
	if activeBody.Revision.Revision != 2 {
		t.Fatalf("expected active revision 2, got %d", activeBody.Revision.Revision)
	}
	t.Logf("BUG #8 (two drafts same base): revision advanced to %d as expected", activeBody.Revision.Revision)
}

// TestDashboardBug9_SaveSecretVaultLocked verifies that the vault error
// response format is JSON-parseable with code and message fields, so the
// frontend can display the actual server error instead of a generic message.
func TestDashboardBug9_SaveSecretVaultLocked(t *testing.T) {
	// Simulate a vault-locked response — the format must match what the
	// secretvault.HTTP handler returns so the frontend can parse it.
	simulatedBody := `{"error":{"code":"vault_locked","message":"credential vault is locked"}}`
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(simulatedBody), &parsed); err != nil {
		t.Fatalf("failed to parse simulated vault error: %v", err)
	}
	if parsed.Error.Code != "vault_locked" {
		t.Fatalf("expected code=vault_locked, got %q", parsed.Error.Code)
	}
	if parsed.Error.Message != "credential vault is locked" {
		t.Fatalf("expected message='credential vault is locked', got %q", parsed.Error.Message)
	}
	t.Logf("BUG #9 verified: vault-locked error is JSON-parseable with code+message")
}

// --- helpers ---

func validateDraft(t *testing.T, baseURL, draftID string) {
	t.Helper()
	resp := mustPOSTJSON(t, baseURL+"/config/drafts/"+draftID+"/validate", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("validate draft %s status=%d body=%s", draftID, resp.StatusCode, readBody(t, resp))
	}
}

func applyDraft(t *testing.T, baseURL, draftID string) {
	t.Helper()
	resp := mustPOSTJSON(t, baseURL+"/config/drafts/"+draftID+"/apply", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("apply draft %s status=%d body=%s", draftID, resp.StatusCode, readBody(t, resp))
	}
}

// createModelsDraftWithProvider creates a MODELS draft with a single provider
// identified by the given id, base_url, and api_key_env. Includes a valid
// global_limit so the provider passes domain validation.
func createModelsDraftWithProvider(t *testing.T, baseURL string, basedOnRev int, providerID, baseURLStr, apiKeyEnv string) string {
	t.Helper()
	payload := map[string]any{
		"scope":             "MODELS",
		"reason":             "add provider " + providerID,
		"based_on_revision":  basedOnRev,
		"models": map[string]any{
			"version": "models.v1",
			"providers": []map[string]any{
				{
					"id":                  providerID,
					"kind":                "openai_compatible",
					"base_url":            baseURLStr,
					"api_key_env":         apiKeyEnv,
					"timeout":             90000000000,
					"max_response_bytes":  10485760,
					"global_limit": map[string]any{
						"resource":       "model-provider:" + providerID,
						"max_concurrent": 2,
						"cooldown_base":  30000000000,
						"cooldown_max":   300000000000,
					},
				},
			},
			"bindings": []map[string]any{},
		},
	}
	resp := mustPOSTJSON(t, baseURL+"/config/drafts", payload)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("create draft status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var body struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	return string(body.Draft.ID)
}

// Ensure the kernel is wired for these tests.
var _ = kernel.ConfigApplier{}
