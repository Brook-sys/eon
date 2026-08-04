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

func TestVaultRefreshToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")

	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer v.Close()

	if err := v.Initialize("secret-pass-long-enough-1234"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// 1. Refresh non-existent token (create)
	err = v.RefreshToken("api/token", "val-1", 1*time.Hour)
	if err != nil {
		t.Fatalf("RefreshToken create failed: %v", err)
	}

	val, err := v.Resolve("api/token")
	if err != nil || val != "val-1" {
		t.Fatalf("Resolve expected val-1, got %q, err: %v", val, err)
	}

	meta, err := v.SecretMetadata("api/token")
	if err != nil {
		t.Fatalf("SecretMetadata failed: %v", err)
	}
	if meta.ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt should not be zero")
	}

	// 2. Refresh existing token (update)
	err = v.RefreshToken("api/token", "val-2", 2*time.Hour)
	if err != nil {
		t.Fatalf("RefreshToken update failed: %v", err)
	}

	val, err = v.Resolve("api/token")
	if err != nil || val != "val-2" {
		t.Fatalf("Resolve expected val-2, got %q, err: %v", val, err)
	}

	// 3. Validation errors
	if err := v.RefreshToken("bad/../name", "val", 0); err == nil {
		t.Fatalf("Expected error for bad name")
	}
	if err := v.RefreshToken("api/token", "val", -10*time.Minute); err != ErrInvalidTTL {
		t.Fatalf("Expected ErrInvalidTTL for negative TTL, got %v", err)
	}

	// 4. Locked vault
	v.Lock()
	if err := v.RefreshToken("api/token", "val-3", 0); err != ErrLocked {
		t.Fatalf("Expected ErrLocked, got %v", err)
	}
}

func TestVaultBatchRefreshToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")

	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer v.Close()

	if err := v.Initialize("secret-pass-long-enough-1234"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	items := []TokenRefreshItem{
		{Name: "token/a", NewValue: "secret-a", TTL: "30m"},
		{Name: "token/b", NewValue: "secret-b", TTL: ""},
		{Name: "invalid/../name", NewValue: "x"},
		{Name: "token/a", NewValue: "duplicate"},
	}

	res, err := v.BatchRefreshToken(items)
	if err != nil {
		t.Fatalf("BatchRefreshToken failed: %v", err)
	}

	if res.Total != 4 {
		t.Fatalf("Expected Total 4, got %d", res.Total)
	}
	if len(res.Refreshed) != 2 || res.Refreshed[0] != "token/a" || res.Refreshed[1] != "token/b" {
		t.Fatalf("Unexpected Refreshed list: %v", res.Refreshed)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("Expected 2 errors in batch, got %d: %v", len(res.Errors), res.Errors)
	}

	valA, _ := v.Resolve("token/a")
	if valA != "secret-a" {
		t.Fatalf("Expected secret-a for token/a, got %q", valA)
	}
}

func TestHTTPRefreshTokenEndpoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")

	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer v.Close()

	if err := v.Initialize("secret-pass-long-enough-1234"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	handler := HTTP{Vault: v}.Handler()

	// 1. POST /secrets/{name}/refresh
	body, _ := json.Marshal(map[string]any{
		"new_value": "fresh-token-123",
		"ttl":       "1h",
	})
	req := httptest.NewRequest("POST", "/secrets/api-token/refresh", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /secrets/{name}/refresh expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	val, _ := v.Resolve("api-token")
	if val != "fresh-token-123" {
		t.Fatalf("Expected fresh-token-123, got %q", val)
	}

	// 2. POST /secrets/batch-refresh
	batchBody, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"name": "service/tok-1", "new_value": "v1", "ttl": "30m"},
			{"name": "service/tok-2", "new_value": "v2"},
		},
	})
	req2 := httptest.NewRequest("POST", "/secrets/batch-refresh", bytes.NewReader(batchBody))
	req2.RemoteAddr = "127.0.0.1:12345"
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("POST /secrets/batch-refresh expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var batchRes TokenRefreshResult
	json.NewDecoder(rec2.Body).Decode(&batchRes)
	if batchRes.Total != 2 || len(batchRes.Refreshed) != 2 {
		t.Fatalf("Unexpected batchRes: %+v", batchRes)
	}
}
