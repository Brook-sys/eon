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

func TestVaultBatchRename(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "batch_rename.json")

	fixedTime := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedTime }

	v, err := NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	// 1. Rejection on locked vault
	_, err = v.BatchRename([]BatchRenameItem{{Source: "a", Destination: "b"}})
	if err != ErrLocked {
		t.Fatalf("BatchRename on locked vault expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("masterpass123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Populate test secrets
	_ = v.Put("app/db/pass", "secret1")
	_ = v.Put("app/redis/pass", "secret2")
	_ = v.Put("app/old/token", "secret3")

	// 2. BatchRename with valid items, missing source, invalid names, and duplicate destination
	items := []BatchRenameItem{
		{Source: "app/db/pass", Destination: "prod/db/pass"},
		{Source: "app/redis/pass", Destination: "prod/redis/pass"},
		{Source: "missing/key", Destination: "prod/missing"},
		{Source: "app/old/token", Destination: "prod/db/pass"}, // duplicate destination
		{Source: "", Destination: "empty/src"},
	}

	res, err := v.BatchRename(items)
	if err != nil {
		t.Fatalf("BatchRename failed: %v", err)
	}

	expectedRenamed := []string{"prod/db/pass", "prod/redis/pass"}
	if !reflect.DeepEqual(res.Renamed, expectedRenamed) {
		t.Fatalf("Expected renamed %v, got %v", expectedRenamed, res.Renamed)
	}

	if len(res.Errors) != 3 {
		t.Fatalf("Expected 3 errors in batch, got %d: %+v", len(res.Errors), res.Errors)
	}

	// Verify vault state
	val, err := v.Resolve("prod/db/pass")
	if err != nil || val != "secret1" {
		t.Fatalf("Expected prod/db/pass='secret1', got val=%q err=%v", val, err)
	}
	val, err = v.Resolve("prod/redis/pass")
	if err != nil || val != "secret2" {
		t.Fatalf("Expected prod/redis/pass='secret2', got val=%q err=%v", val, err)
	}
	if _, err := v.Resolve("app/db/pass"); err == nil {
		t.Fatalf("Source app/db/pass should no longer exist after rename")
	}

	// 3. Self-rename (metadata refresh)
	resSelf, err := v.BatchRename([]BatchRenameItem{{Source: "prod/db/pass", Destination: "prod/db/pass"}})
	if err != nil {
		t.Fatalf("Self BatchRename failed: %v", err)
	}
	if len(resSelf.Renamed) != 1 || resSelf.Renamed[0] != "prod/db/pass" {
		t.Fatalf("Self BatchRename failed result: %+v", resSelf)
	}
}

func TestHTTPBatchRename(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "http_batch_rename.json")

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpass123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	_ = v.Put("service/key1", "val1")

	handler := HTTP{Vault: v}.Handler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := map[string]any{
		"items": []map[string]string{
			{"source": "service/key1", "destination": "service/key1_renamed"},
		},
	}
	reqBody, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/secrets/batch-rename", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /secrets/batch-rename failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d", resp.StatusCode)
	}

	var res BatchRenameResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Decode BatchRenameResult failed: %v", err)
	}

	if len(res.Renamed) != 1 || res.Renamed[0] != "service/key1_renamed" {
		t.Fatalf("Expected renamed ['service/key1_renamed'], got %v", res.Renamed)
	}
}
