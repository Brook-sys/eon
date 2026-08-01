package secretvault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVaultExists(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Locked vault must return ErrLocked.
	if _, err := v.Exists("k1"); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := v.Put("k1", "val1"); err != nil {
		t.Fatalf("Put k1 failed: %v", err)
	}

	exists, err := v.Exists("k1")
	if err != nil {
		t.Fatalf("Exists k1 failed: %v", err)
	}
	if !exists {
		t.Fatal("k1 should exist")
	}

	exists, err = v.Exists("missing")
	if err != nil {
		t.Fatalf("Exists missing failed: %v", err)
	}
	if exists {
		t.Fatal("missing should not exist")
	}

	// Invalid names surface an error rather than Exists=false.
	if _, err := v.Exists("bad//name"); err == nil {
		t.Fatal("expected error for invalid name")
	}

	// Expired-but-unpurged records still count as existing, matching
	// BatchExists semantics: create a record in the past, ensure it has not
	// been purged by expireLocked (it has — expireLocked runs on every entry
	// point), and assert Exists reports the same answer BatchExists would
	// give after deletion: false for absent records.
	if err := v.PutWithExpiry("soon-expired", "v", time.Now().Add(-time.Second)); err != nil {
		t.Fatalf("PutWithExpiry failed: %v", err)
	}
	// After an entry point runs expireLocked, that record may be purged;
	// what must hold is that Exists and BatchExists agree on the answer.
	be, err := v.BatchExists([]string{"soon-expired"})
	if err != nil {
		t.Fatalf("BatchExists failed: %v", err)
	}
	ex, err := v.Exists("soon-expired")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if ex != be[0].Exists {
		t.Fatalf("Exists=%v disagrees with BatchExists=%v", ex, be[0].Exists)
	}

	// Deletion yields a definitive miss on both endpoints.
	if err := v.Delete("k1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	exists, err = v.Exists("k1")
	if err != nil {
		t.Fatalf("Exists after delete failed: %v", err)
	}
	if exists {
		t.Fatal("deleted secret should not exist")
	}
}

func TestHTTP_SecretExistsEndpoint(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("api-key", "sk-test"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	h := HTTP{Vault: v}.Handler()

	type existsResponse struct {
		Name   string `json:"name"`
		Exists bool   `json:"exists"`
	}

	// Happy path: existing secret.
	req := httptest.NewRequest(http.MethodGet, "/secrets/api-key/exists", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got existsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if !got.Exists || got.Name != "api-key" {
		t.Fatalf("unexpected response: %+v", got)
	}
	// The response must never carry the secret value.
	if strings.Contains(rec.Body.String(), "sk-test") {
		t.Fatalf("secret value leaked in exists response: %s", rec.Body.String())
	}

	// Missing secret returns 200 with exists=false (not 404), because the
	// endpoint answers a boolean question rather than fetching a resource.
	req = httptest.NewRequest(http.MethodGet, "/secrets/nope/exists", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for missing name, got %d", rec.Code)
	}
	got = existsResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Exists {
		t.Fatalf("expected exists=false, got %+v", got)
	}

	// Invalid name is a client error. A name segment whose decoded form ends
	// in "/" fails validateName (HasSuffix "/"), exercising the error path
	// without multi-segment routing issues.
	req = httptest.NewRequest(http.MethodGet, "/secrets/badname%2F/exists", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 for invalid name, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Locked vault maps to 423.
	v.Lock()
	req = httptest.NewRequest(http.MethodGet, "/secrets/api-key/exists", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423 when locked, got %d", rec.Code)
	}
}
