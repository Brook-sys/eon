package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTouchSingleSecret(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	if err := v.Initialize("pass1234567890"); err != nil {
		t.Fatal(err)
	}

	// Touch non-existent secret -> os.ErrNotExist
	if err := v.Touch("app/config", time.Hour); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	// Put secret with short TTL
	if err := v.PutWithTTL("app/config", "secret1", time.Minute); err != nil {
		t.Fatal(err)
	}

	// Extend TTL to 2 hours
	if err := v.Touch("app/config", 2*time.Hour); err != nil {
		t.Fatalf("unexpected touch error: %v", err)
	}

	meta, err := v.SecretMetadata("app/config")
	if err != nil {
		t.Fatal(err)
	}
	if meta.ExpiresAt.IsZero() {
		t.Fatal("expected non-zero ExpiresAt")
	}

	// Clear TTL by passing 0 duration
	if err := v.Touch("app/config", 0); err != nil {
		t.Fatalf("unexpected touch error clearing ttl: %v", err)
	}
	meta, err = v.SecretMetadata("app/config")
	if err != nil {
		t.Fatal(err)
	}
	if !meta.ExpiresAt.IsZero() {
		t.Fatalf("expected zero ExpiresAt, got %v", meta.ExpiresAt)
	}

	// Touch on locked vault -> ErrLocked
	v.Lock()
	if err := v.Touch("app/config", time.Hour); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestBulkTouchAndHTTP(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	if err := v.Initialize("pass1234567890"); err != nil {
		t.Fatal(err)
	}

	if err := v.Put("sec1", "val1"); err != nil {
		t.Fatal(err)
	}
	if err := v.PutWithTTL("sec2", "val2", time.Minute); err != nil {
		t.Fatal(err)
	}

	// BulkTouch with mix of valid and invalid
	items := []BulkTouchItem{
		{Name: "sec1", TTL: time.Hour},
		{Name: "sec2", TTL: 30 * time.Minute},
		{Name: "sec3", TTL: time.Hour}, // not found
		{Name: "invalid name!", TTL: time.Hour},
	}

	res, err := v.BulkTouch(items)
	if err != nil {
		t.Fatalf("unexpected BulkTouch error: %v", err)
	}

	if len(res.Updated) != 2 || res.Updated[0] != "sec1" || res.Updated[1] != "sec2" {
		t.Fatalf("unexpected Updated slice: %v", res.Updated)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %v", res.Errors)
	}

	// HTTP test
	handler := HTTP{Vault: v}.Handler()

	body, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"name": "sec1", "ttl": 7200000000000}, // 2h in ns
		},
	})
	req := httptest.NewRequest("POST", "/secrets/bulk-touch", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var httpRes BulkTouchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &httpRes); err != nil {
		t.Fatal(err)
	}
	if len(httpRes.Updated) != 1 || httpRes.Updated[0] != "sec1" {
		t.Fatalf("unexpected HTTP response: %+v", httpRes)
	}
}
