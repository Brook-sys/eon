package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestVault_BatchSecretHistory(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault_batch_history.json")

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer v.Close()

	if err := v.Initialize("secret-password-123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Locked vault returns ErrLocked
	v.Lock()
	if _, err := v.BatchSecretHistory([]string{"k1"}); err != ErrLocked {
		t.Fatalf("expected ErrLocked when locked, got %v", err)
	}

	if err := v.Unlock("secret-password-123"); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	// Setup secrets and actions
	_ = v.Put("k1", "val1")
	_ = v.Rotate("k1", "val1_new")
	_ = v.Put("k2", "val2")

	// Batch history query
	results, err := v.BatchSecretHistory([]string{"k1", "k2", "missing", "invalid/../name"})
	if err != nil {
		t.Fatalf("BatchSecretHistory failed: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Check k1
	if !results[0].Found || len(results[0].History) < 2 {
		t.Errorf("k1 expected found with >= 2 events, got %+v", results[0])
	}
	// Check k2
	if !results[1].Found || len(results[1].History) < 1 {
		t.Errorf("k2 expected found with >= 1 event, got %+v", results[1])
	}
	// Check missing
	if results[2].Found || results[2].Error == "" {
		t.Errorf("missing expected found=false with error, got %+v", results[2])
	}
	// Check invalid name
	if results[3].Error == "" {
		t.Errorf("invalid name expected error, got %+v", results[3])
	}
}

func TestHTTP_BatchSecretHistory(t *testing.T) {
	dir := t.TempDir()
	v, err := New(filepath.Join(dir, "vault_http_batch_history.json"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer v.Close()

	_ = v.Initialize("pass123-long-enough")
	_ = v.Unlock("pass123-long-enough")
	_ = v.Put("token1", "v1")

	srv := httptest.NewServer(HTTP{Vault: v}.Handler())
	defer srv.Close()

	// Empty request body -> 400
	reqBody, _ := json.Marshal(batchHistoryRequest{Names: []string{}})
	resp, err := http.Post(srv.URL+"/secrets/batch-history", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("HTTP POST /secrets/batch-history failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty names, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Valid batch request
	reqBody, _ = json.Marshal(batchHistoryRequest{Names: []string{"token1", "missing_token"}})
	resp, err = http.Post(srv.URL+"/secrets/batch-history", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("HTTP POST /secrets/batch-history failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var res []BatchSecretHistoryResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp.Body.Close()

	if len(res) != 2 {
		t.Fatalf("expected 2 items in result, got %d", len(res))
	}
	if !res[0].Found || res[0].Name != "token1" {
		t.Errorf("unexpected res[0]: %+v", res[0])
	}
	if res[1].Found || res[1].Name != "missing_token" {
		t.Errorf("unexpected res[1]: %+v", res[1])
	}
}
