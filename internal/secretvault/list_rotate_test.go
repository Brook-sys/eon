package secretvault

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestVaultListSecrets(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// ListSecrets on locked vault returns ErrLocked.
	if _, err := v.ListSecrets(); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Empty vault lists no secrets.
	entries, err := v.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets on empty vault failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}

	// Populate with multiple secrets.
	if err := v.Put("zebra_key", "val1"); err != nil {
		t.Fatalf("Put zebra_key failed: %v", err)
	}
	if err := v.Put("alpha_key", "val2"); err != nil {
		t.Fatalf("Put alpha_key failed: %v", err)
	}
	if err := v.PutWithTTL("temp_key", "val3", 10*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL temp_key failed: %v", err)
	}

	// Wait for temp_key to expire.
	time.Sleep(20 * time.Millisecond)

	entries, err = v.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify sorted by name.
	if entries[0].Name != "alpha_key" {
		t.Fatalf("expected alpha_key first, got %s", entries[0].Name)
	}
	if entries[1].Name != "temp_key" {
		t.Fatalf("expected temp_key second, got %s", entries[1].Name)
	}
	if entries[2].Name != "zebra_key" {
		t.Fatalf("expected zebra_key third, got %s", entries[2].Name)
	}

	// Verify expired flag on temp_key.
	if !entries[1].Expired {
		t.Fatalf("expected temp_key to be expired")
	}
	if entries[0].Expired {
		t.Fatalf("expected alpha_key to not be expired")
	}

	// Verify metadata fields are populated.
	for _, e := range entries {
		if e.CreatedAt.IsZero() {
			t.Fatalf("entry %s has zero CreatedAt", e.Name)
		}
		if e.UpdatedAt.IsZero() {
			t.Fatalf("entry %s has zero UpdatedAt", e.Name)
		}
	}
}

func TestVaultRotate(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Rotate on locked vault returns ErrLocked.
	if err := v.Rotate("any_key", "newval"); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("api_key", "old_secret_value"); err != nil {
		t.Fatalf("Put api_key failed: %v", err)
	}

	// Record CreatedAt before rotation.
	originalRecord := v.data.Secrets["api_key"]
	originalCreatedAt := originalRecord.CreatedAt

	// Rotate non-existent secret returns os.ErrNotExist.
	if err := v.Rotate("missing_key", "newval"); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	// Rotate with empty value returns ErrInvalidSecretValue.
	if err := v.Rotate("api_key", ""); err != ErrInvalidSecretValue {
		t.Fatalf("expected ErrInvalidSecretValue, got %v", err)
	}

	// Rotate with valid new value succeeds.
	if err := v.Rotate("api_key", "new_secret_value"); err != nil {
		t.Fatalf("Rotate api_key failed: %v", err)
	}

	// Verify new value resolves.
	val, err := v.Resolve("api_key")
	if err != nil || val != "new_secret_value" {
		t.Fatalf("expected new_secret_value, got %q, err: %v", val, err)
	}

	// Verify CreatedAt was preserved and UpdatedAt advanced.
	rotatedRecord := v.data.Secrets["api_key"]
	if !rotatedRecord.CreatedAt.Equal(originalCreatedAt) {
		t.Fatalf("CreatedAt should be preserved after rotation; original=%v rotated=%v", originalCreatedAt, rotatedRecord.CreatedAt)
	}
	if !rotatedRecord.UpdatedAt.After(originalCreatedAt) {
		t.Fatalf("UpdatedAt should be after original CreatedAt; original=%v rotated=%v", originalCreatedAt, rotatedRecord.UpdatedAt)
	}
}

func TestHTTPListAndRotate(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("api_key", "original"); err != nil {
		t.Fatalf("Put api_key failed: %v", err)
	}
	if err := v.Put("db_pass", "secret123"); err != nil {
		t.Fatalf("Put db_pass failed: %v", err)
	}

	h := HTTP{Vault: v}.Handler()

	// GET /secrets -> 200 OK with list of secret metadata (sorted by name)
	req := httptest.NewRequest("GET", "/secrets", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Must contain both secret names
	if !strings.Contains(body, "api_key") {
		t.Fatalf("response missing 'api_key': %s", body)
	}
	if !strings.Contains(body, "db_pass") {
		t.Fatalf("response missing 'db_pass': %s", body)
	}
	// Must NOT contain secret values
	if strings.Contains(body, "original") {
		t.Fatalf("response leaked 'original' value: %s", body)
	}
	if strings.Contains(body, "secret123") {
		t.Fatalf("response leaked 'secret123' value: %s", body)
	}

	// POST /secrets/api_key/rotate -> 204 No Content
	req = httptest.NewRequest("POST", "/secrets/api_key/rotate", strings.NewReader(`{"value":"rotated_value"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify new value resolves via GET /secrets/api_key
	req = httptest.NewRequest("GET", "/secrets/api_key", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "rotated_value") {
		t.Fatalf("expected rotated_value in response: %s", rec.Body.String())
	}

	// POST /secrets/nonexistent/rotate -> 404 Not Found
	req = httptest.NewRequest("POST", "/secrets/nonexistent/rotate", strings.NewReader(`{"value":"x"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
	}

	// POST /secrets/api_key/rotate with empty value -> 400 Bad Request
	req = httptest.NewRequest("POST", "/secrets/api_key/rotate", strings.NewReader(`{"value":""}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
	}

	// Lock vault and verify GET /secrets -> 423 Locked
	v.Lock()
	req = httptest.NewRequest("GET", "/secrets", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423 Locked, got %d", rec.Code)
	}

	// POST /secrets/api_key/rotate on locked vault -> 423 Locked
	req = httptest.NewRequest("POST", "/secrets/api_key/rotate", strings.NewReader(`{"value":"x"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423 Locked, got %d", rec.Code)
	}
}
