package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestDeleteAllAndPurgeBatch(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	now := time.Now().UTC()

	v, err := NewWithClock(vaultPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWithClock: %v", err)
	}

	// 1. DeleteAll on uninitialized/locked vault -> ErrLocked
	if _, err := v.DeleteAll(); err != ErrLocked {
		t.Errorf("expected ErrLocked on uninitialized vault, got: %v", err)
	}

	if err := v.Initialize("MasterPassword123!"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// DeleteAll on empty vault -> 0
	cnt, err := v.DeleteAll()
	if err != nil {
		t.Fatalf("DeleteAll empty: %v", err)
	}
	if cnt != 0 {
		t.Errorf("expected 0 deleted, got %d", cnt)
	}

	// Populate secrets
	if err := v.Put("sec1", "val1"); err != nil {
		t.Fatalf("Put sec1: %v", err)
	}
	if err := v.Put("sec2", "val2"); err != nil {
		t.Fatalf("Put sec2: %v", err)
	}

	// Test DeleteAll method
	cnt, err = v.DeleteAll()
	if err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if cnt != 2 {
		t.Errorf("expected 2 deleted, got %d", cnt)
	}

	entries, err := v.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 secrets remaining, got %d", len(entries))
	}

	// Populate again for HTTP test
	if err := v.Put("app/db_pass", "secret1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := v.Put("app/api_key", "secret2"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	handler := HTTP{Vault: v}.Handler()

	// HTTP DELETE /secrets -> 200 OK {"deleted": 2}
	req := httptest.NewRequest("DELETE", "/secrets", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 for DELETE /secrets, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}
	if resp["deleted"] != 2 {
		t.Errorf("expected deleted=2 in HTTP response, got %d", resp["deleted"])
	}

	// Verify vault file on disk is updated and empty
	v.Lock()
	if err := v.Unlock("MasterPassword123!"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	entries, err = v.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets after unlock: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 secrets after HTTP DELETE /secrets, got %d", len(entries))
	}
}

func TestResolveBatchLimitValidation(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.Initialize("MasterPassword123!"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	handler := HTTP{Vault: v}.Handler()

	// Empty names array -> HTTP 400
	body, _ := json.Marshal(map[string]any{"names": []string{}})
	req := httptest.NewRequest("POST", "/resolve", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for empty names, got %d", rec.Code)
	}

	// Oversized names array (>100) -> HTTP 400
	names := make([]string, 101)
	for i := range names {
		names[i] = "sec"
	}
	body, _ = json.Marshal(map[string]any{"names": names})
	req = httptest.NewRequest("POST", "/resolve", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for >100 names, got %d", rec.Code)
	}
}
