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

func TestVault_BatchPut(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "batch_put_vault.json")

	fixedTime := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedTime }

	v, err := NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	// BatchPut on locked vault should return ErrLocked.
	if _, err := v.BatchPut([]BatchPutItem{{Name: "k1", Value: "v1"}}); err != ErrLocked {
		t.Fatalf("BatchPut on locked vault expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("master-password-123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Pre-populate key1 for update check
	if err := v.Put("key1", "old_val"); err != nil {
		t.Fatalf("Put key1 failed: %v", err)
	}

	items := []BatchPutItem{
		{Name: "key1", Value: "new_val"},                       // update
		{Name: "key2", Value: "val2"},                          // create
		{Name: "key3", Value: "val3", TTL: 5 * time.Minute},    // create with TTL
		{Name: "invalid/../name", Value: "val"},                // invalid name
		{Name: "empty_val", Value: ""},                         // invalid value (empty)
		{Name: "neg_ttl", Value: "val", TTL: -1 * time.Minute}, // negative TTL
	}

	res, err := v.BatchPut(items)
	if err != nil {
		t.Fatalf("BatchPut failed: %v", err)
	}

	if len(res.Created) != 2 || res.Created[0] != "key2" || res.Created[1] != "key3" {
		t.Fatalf("unexpected Created list: %+v", res.Created)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "key1" {
		t.Fatalf("unexpected Updated list: %+v", res.Updated)
	}
	if len(res.Stored) != 3 || res.Stored[0] != "key1" || res.Stored[1] != "key2" || res.Stored[2] != "key3" {
		t.Fatalf("unexpected Stored list: %+v", res.Stored)
	}
	if len(res.Errors) != 3 {
		t.Fatalf("expected 3 errors, got %d: %+v", len(res.Errors), res.Errors)
	}

	// Verify values and expiration
	val, err := v.Resolve("key1")
	if err != nil || val != "new_val" {
		t.Fatalf("key1 value expected 'new_val', got %q (err=%v)", val, err)
	}
	val, err = v.Resolve("key2")
	if err != nil || val != "val2" {
		t.Fatalf("key2 value expected 'val2', got %q (err=%v)", val, err)
	}

	// Advance time past TTL of key3 (but before auto-lock of 15m) and verify expiration
	fixedTime = fixedTime.Add(10 * time.Minute)
	_, err = v.Resolve("key3")
	if err != ErrSecretExpired {
		t.Fatalf("key3 expected ErrSecretExpired after TTL, got %v", err)
	}

	// Durability check: reopen vault and verify state
	v2, err := NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock v2 failed: %v", err)
	}
	if err := v2.Unlock("master-password-123"); err != nil {
		t.Fatalf("Unlock v2 failed: %v", err)
	}
	val, err = v2.Resolve("key1")
	if err != nil || val != "new_val" {
		t.Fatalf("v2 key1 value expected 'new_val', got %q (err=%v)", val, err)
	}
}

func TestHTTP_BatchPut(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "http_batch_put_vault.json")

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpass123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	handler := HTTP{Vault: v}.Handler()

	// Invalid payload: empty items array
	body, _ := json.Marshal(map[string]any{
		"items": []any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/secrets/batch-put", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /secrets/batch-put empty items expected HTTP 400, got %d", rec.Code)
	}

	// Valid payload
	body, _ = json.Marshal(map[string]any{
		"items": []map[string]any{
			{"name": "app/db/host", "value": "localhost"},
			{"name": "app/db/port", "value": "5432"},
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/secrets/batch-put", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /secrets/batch-put returned code %d: %s", rec.Code, rec.Body.String())
	}

	var putRes BatchPutResult
	if err := json.Unmarshal(rec.Body.Bytes(), &putRes); err != nil {
		t.Fatalf("Unmarshal batch put response failed: %v", err)
	}
	if len(putRes.Created) != 2 || len(putRes.Stored) != 2 {
		t.Fatalf("expected 2 created/stored secrets, got %+v", putRes)
	}
}
