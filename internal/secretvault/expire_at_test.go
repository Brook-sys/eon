package secretvault

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpireAtSetsAndClearsAbsoluteExpiry(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	if err := v.Initialize("pass1234567890"); err != nil {
		t.Fatal(err)
	}

	// ExpireAt on a missing secret -> os.ErrNotExist
	if err := v.ExpireAt("missing", time.Now().Add(time.Hour)); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	if err := v.Put("app/token", "value1"); err != nil {
		t.Fatal(err)
	}

	// Pin an absolute deadline (non-UTC input must be normalized to UTC).
	loc := time.FixedZone("UTC+5", 5*3600)
	deadline := time.Date(2027, 1, 15, 12, 0, 0, 0, loc)
	if err := v.ExpireAt("app/token", deadline); err != nil {
		t.Fatalf("unexpected ExpireAt error: %v", err)
	}
	meta, err := v.SecretMetadata("app/token")
	if err != nil {
		t.Fatal(err)
	}
	want := deadline.UTC()
	if !meta.ExpiresAt.Equal(want) {
		t.Fatalf("expected ExpiresAt %v, got %v", want, meta.ExpiresAt)
	}

	// Clear by passing zero time.
	if err := v.ExpireAt("app/token", time.Time{}); err != nil {
		t.Fatalf("unexpected ExpireAt clear error: %v", err)
	}
	meta, err = v.SecretMetadata("app/token")
	if err != nil {
		t.Fatal(err)
	}
	if !meta.ExpiresAt.IsZero() {
		t.Fatalf("expected zero ExpiresAt, got %v", meta.ExpiresAt)
	}

	// Invalid name rejected with a validation error distinct from not-found.
	if err := v.ExpireAt("/leading-slash", time.Now()); !errors.Is(err, ErrInvalidSecretName) {
		t.Fatalf("expected ErrInvalidSecretName, got %v", err)
	}

	// Locked vault -> ErrLocked
	v.Lock()
	if err := v.ExpireAt("app/token", time.Now().Add(time.Hour)); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestBatchExpireAtMixedValidity(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	if err := v.Initialize("pass1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("sec1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := v.PutWithTTL("sec2", "v2", time.Minute); err != nil {
		t.Fatal(err)
	}

	when := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	res, err := v.BatchExpireAt([]BatchExpireAtItem{
		{Name: "sec1", ExpiresAt: when},
		{Name: "sec2", ExpiresAt: time.Time{}}, // clear
		{Name: "sec3", ExpiresAt: when},        // does not exist
		{Name: "invalid//name", ExpiresAt: when},
	})
	if err != nil {
		t.Fatalf("unexpected BatchExpireAt error: %v", err)
	}
	if len(res.Updated) != 2 || res.Updated[0] != "sec1" || res.Updated[1] != "sec2" {
		t.Fatalf("unexpected Updated slice: %v", res.Updated)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %v", res.Errors)
	}

	meta1, err := v.SecretMetadata("sec1")
	if err != nil {
		t.Fatal(err)
	}
	if !meta1.ExpiresAt.Equal(when) {
		t.Fatalf("sec1 ExpiresAt: want %v got %v", when, meta1.ExpiresAt)
	}
	meta2, err := v.SecretMetadata("sec2")
	if err != nil {
		t.Fatal(err)
	}
	if !meta2.ExpiresAt.IsZero() {
		t.Fatalf("sec2 ExpiresAt not cleared: %v", meta2.ExpiresAt)
	}

	// All-invalid batch: no update, no error.
	res2, err := v.BatchExpireAt([]BatchExpireAtItem{{Name: "nope", ExpiresAt: when}})
	if err != nil {
		t.Fatalf("unexpected all-invalid batch error: %v", err)
	}
	if len(res2.Updated) != 0 || len(res2.Errors) != 1 {
		t.Fatalf("unexpected all-invalid result: %+v", res2)
	}

	// Locked vault -> ErrLocked
	v.Lock()
	if _, err := v.BatchExpireAt([]BatchExpireAtItem{{Name: "sec1", ExpiresAt: when}}); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestBatchExpireAtPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	if err := v.Initialize("pass1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("sec1", "v1"); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2028, 3, 3, 3, 3, 3, 0, time.UTC)
	if _, err := v.BatchExpireAt([]BatchExpireAtItem{{Name: "sec1", ExpiresAt: when}}); err != nil {
		t.Fatal(err)
	}
	v.Lock()

	// Reopen the same vault file; the expiry must survive the round trip.
	v2, _ := New(vaultPath)
	if err := v2.Unlock("pass1234567890"); err != nil {
		t.Fatal(err)
	}
	meta, err := v2.SecretMetadata("sec1")
	if err != nil {
		t.Fatal(err)
	}
	if !meta.ExpiresAt.Equal(when) {
		t.Fatalf("round-trip ExpiresAt: want %v got %v", when, meta.ExpiresAt)
	}
}

func TestHTTPExpireAtEndpoints(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, _ := New(vaultPath)
	if err := v.Initialize("pass1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("sec1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("sec2", "v2"); err != nil {
		t.Fatal(err)
	}
	handler := HTTP{Vault: v}.Handler()

	// Single-secret endpoint: 204 and expiry applied.
	when := time.Date(2027, 2, 2, 2, 2, 2, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{"expires_at": when})
	req := httptest.NewRequest(http.MethodPost, "/secrets/sec1/expire-at", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	meta, err := v.SecretMetadata("sec1")
	if err != nil {
		t.Fatal(err)
	}
	if !meta.ExpiresAt.Equal(when) {
		t.Fatalf("HTTP expire-at: want %v got %v", when, meta.ExpiresAt)
	}

	// Missing secret via HTTP -> 404.
	body, _ = json.Marshal(map[string]any{"expires_at": when})
	req = httptest.NewRequest(http.MethodPost, "/secrets/ghost/expire-at", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	// Batch endpoint: partial success reported in payload.
	body, _ = json.Marshal(map[string]any{"items": []map[string]any{
		{"name": "sec2", "expires_at": when},
		{"name": "ghost", "expires_at": when},
	}})
	req = httptest.NewRequest(http.MethodPost, "/secrets/batch-expire-at", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var br BatchExpireAtResult
	if err := json.Unmarshal(rec.Body.Bytes(), &br); err != nil {
		t.Fatal(err)
	}
	if len(br.Updated) != 1 || br.Updated[0] != "sec2" {
		t.Fatalf("unexpected batch Updated: %+v", br)
	}
	if len(br.Errors) != 1 {
		t.Fatalf("unexpected batch Errors: %+v", br)
	}

	// Batch limits: empty items -> 400.
	body, _ = json.Marshal(map[string]any{"items": []map[string]any{}})
	req = httptest.NewRequest(http.MethodPost, "/secrets/batch-expire-at", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty items, got %d: %s", rec.Code, rec.Body.String())
	}

	// Batch limits: more than 100 items -> 400.
	items := make([]map[string]any, 0, 101)
	for i := 0; i < 101; i++ {
		items = append(items, map[string]any{"name": "sec1", "expires_at": when})
	}
	body, _ = json.Marshal(map[string]any{"items": items})
	req = httptest.NewRequest(http.MethodPost, "/secrets/batch-expire-at", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 101 items, got %d: %s", rec.Code, rec.Body.String())
	}

	// No secret value leaks in any response above.
	if bytes.Contains(rec.Body.Bytes(), []byte("v1")) || bytes.Contains(rec.Body.Bytes(), []byte("v2")) {
		t.Fatal("secret value leaked into expire-at HTTP response")
	}
}
