package secretvault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestVaultHealth(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Uninitialized health check.
	h := v.Health()
	if h.Initialized {
		t.Fatalf("expected Initialized=false")
	}
	if !h.Locked {
		t.Fatalf("expected Locked=true")
	}
	if h.FileExists {
		t.Fatalf("expected FileExists=false")
	}
	if h.TotalSecrets != 0 {
		t.Fatalf("expected TotalSecrets=0")
	}

	// Initialize vault.
	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Initialized and unlocked health check.
	h = v.Health()
	if !h.Initialized || !h.FileExists {
		t.Fatalf("expected Initialized=true and FileExists=true")
	}
	if h.Locked {
		t.Fatalf("expected Locked=false")
	}
	if h.TotalSecrets != 0 {
		t.Fatalf("expected TotalSecrets=0")
	}

	// Put normal secret and expired secret.
	if err := v.Put("k1", "v1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}
	if err := v.PutWithTTL("k2", "v2", 10*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL k2 failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	h = v.Health()
	if h.TotalSecrets != 2 {
		t.Fatalf("expected TotalSecrets=2, got %d", h.TotalSecrets)
	}
	if h.ExpiredSecrets != 1 {
		t.Fatalf("expected ExpiredSecrets=1, got %d", h.ExpiredSecrets)
	}
	if h.AuditEvents < 2 {
		t.Fatalf("expected AuditEvents>=2, got %d", h.AuditEvents)
	}
}

func TestVaultSecretHistory(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// SecretHistory on locked vault returns ErrLocked.
	if _, err := v.SecretHistory("missing"); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Invalid name returns ErrInvalidSecretName.
	if _, err := v.SecretHistory("a/../b"); err != ErrInvalidSecretName {
		t.Fatalf("expected ErrInvalidSecretName, got %v", err)
	}

	// Non-existent secret history returns os.ErrNotExist.
	if _, err := v.SecretHistory("missing"); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	// Perform operations on "db_key": Put, Rotate, Get.
	if err := v.Put("db_key", "val1"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := v.Rotate("db_key", "val2"); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
	if _, err := v.Resolve("db_key"); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	hist, err := v.SecretHistory("db_key")
	if err != nil {
		t.Fatalf("SecretHistory failed: %v", err)
	}
	if len(hist) < 3 {
		t.Fatalf("expected at least 3 audit events for db_key, got %d", len(hist))
	}
}

func TestHTTPHealthAndHistoryEndpoints(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("my_secret", "val"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	srv := httptest.NewServer(HTTP{Vault: v}.Handler())
	defer srv.Close()

	// GET /health
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	var health VaultHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Decode VaultHealth failed: %v", err)
	}
	if !health.Initialized || health.TotalSecrets != 1 {
		t.Fatalf("unexpected VaultHealth payload: %+v", health)
	}

	// GET /secrets/my_secret/history
	resp, err = http.Get(srv.URL + "/secrets/my_secret/history")
	if err != nil {
		t.Fatalf("GET history failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	var histResp struct {
		History []AuditEvent `json:"history"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&histResp); err != nil {
		t.Fatalf("Decode History failed: %v", err)
	}
	if len(histResp.History) == 0 {
		t.Fatalf("expected non-empty history for my_secret")
	}

	// GET /secrets/missing/history -> HTTP 404
	resp, err = http.Get(srv.URL + "/secrets/missing/history")
	if err != nil {
		t.Fatalf("GET missing history failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d", resp.StatusCode)
	}
}
