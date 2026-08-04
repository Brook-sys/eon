package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestVaultBatchPurgeExpired(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "batch_purge.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("secret-pass-long-enough"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer v.Close()

	// Add 2 expired secrets and 2 active secrets
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	future := now.Add(1 * time.Hour)

	if err := v.PutWithExpiry("expired/1", "val1", past); err != nil {
		t.Fatalf("PutWithExpiry expired/1 failed: %v", err)
	}
	if err := v.PutWithExpiry("expired/2", "val2", past); err != nil {
		t.Fatalf("PutWithExpiry expired/2 failed: %v", err)
	}
	if err := v.PutWithExpiry("active/1", "val3", future); err != nil {
		t.Fatalf("PutWithExpiry active/1 failed: %v", err)
	}
	if err := v.Put("active/2", "val4"); err != nil {
		t.Fatalf("Put active/2 failed: %v", err)
	}

	// Test 1: BatchPurgeExpired on subset of names containing expired, active, missing
	targetNames := []string{"expired/1", "expired/2", "active/1", "missing/secret"}
	res, err := v.BatchPurgeExpired(targetNames)
	if err != nil {
		t.Fatalf("BatchPurgeExpired failed: %v", err)
	}

	expectedPurged := []string{"expired/1", "expired/2"}
	if !reflect.DeepEqual(res.Purged, expectedPurged) {
		t.Fatalf("Expected purged %v, got %v", expectedPurged, res.Purged)
	}

	// Verify expired/1 and expired/2 are gone
	exists1, _ := v.Exists("expired/1")
	exists2, _ := v.Exists("expired/2")
	if exists1 || exists2 {
		t.Fatalf("Expired secrets should no longer exist after BatchPurgeExpired")
	}

	// Verify active/1 and active/2 still exist
	active1, _ := v.Exists("active/1")
	active2, _ := v.Exists("active/2")
	if !active1 || !active2 {
		t.Fatalf("Active secrets must not be purged")
	}

	// Test 2: Locked vault rejection
	v.Lock()
	if _, err := v.BatchPurgeExpired(targetNames); err != ErrLocked {
		t.Fatalf("Expected ErrLocked for locked vault, got %v", err)
	}
}

func TestHTTPBatchPurgeExpired(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "http_batch_purge.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("secret-pass-long-enough"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer v.Close()

	past := time.Now().Add(-5 * time.Minute)
	_ = v.PutWithExpiry("tok/expired", "old-val", past)
	_ = v.Put("tok/active", "new-val")

	h := HTTP{Vault: v}
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	reqBody, _ := json.Marshal(map[string]any{
		"names": []string{"tok/expired", "tok/active"},
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/secrets/batch-purge-expired", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /secrets/batch-purge-expired failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}

	var res BatchPurgeResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}

	if len(res.Purged) != 1 || res.Purged[0] != "tok/expired" {
		t.Fatalf("Expected purged ['tok/expired'], got %v", res.Purged)
	}
}
