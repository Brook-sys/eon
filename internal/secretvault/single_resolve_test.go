package secretvault

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestVaultSingleResolve(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 1. Resolve when locked returns ErrLocked.
	if _, err := v.Resolve("db_pass"); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	// 2. Initialize and store secret.
	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("db_pass", "secret123"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 3. Resolve existing secret returns value.
	val, err := v.Resolve("db_pass")
	if err != nil || val != "secret123" {
		t.Fatalf("expected secret123, got %q, err: %v", val, err)
	}

	// 4. Resolve missing secret returns os.ErrNotExist.
	if _, err := v.Resolve("missing_key"); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	// 5. Resolve expired secret returns ErrSecretExpired.
	if err := v.PutWithTTL("temp_token", "val", 10*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := v.Resolve("temp_token"); err != ErrSecretExpired {
		t.Fatalf("expected ErrSecretExpired, got %v", err)
	}
}

func TestHTTPSingleResolveEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("apiKey", "key_abc_123"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	h := HTTP{Vault: v}.Handler()

	// GET /secrets/apiKey -> 200 OK with value
	req := httptest.NewRequest("GET", "/secrets/apiKey", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "{\"value\":\"key_abc_123\"}\n" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}

	// GET /secrets/nonexistent -> 404 Not Found
	req = httptest.NewRequest("GET", "/secrets/nonexistent", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rec.Code)
	}

	// Lock vault and verify GET /secrets/apiKey -> 423 Locked
	v.Lock()
	req = httptest.NewRequest("GET", "/secrets/apiKey", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423 Locked, got %d", rec.Code)
	}
}
