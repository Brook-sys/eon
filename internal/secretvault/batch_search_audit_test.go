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

func TestVault_SearchSecrets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "search_vault.json")

	fixedTime := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedTime }

	v, err := NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}

	// Locked search returns ErrLocked.
	if _, err := v.SearchSecrets("prod/", ""); err != ErrLocked {
		t.Fatalf("SearchSecrets on locked vault expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("master-password-12345"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Populate secrets
	secrets := map[string]string{
		"prod/db/pass":       "secret1",
		"prod/api/token":     "secret2",
		"staging/db/pass":    "secret3",
		"dev/redis/password": "secret4",
		"common/TOKEN":       "secret5",
	}
	for name, val := range secrets {
		if err := v.Put(name, val); err != nil {
			t.Fatalf("Put(%q) failed: %v", name, err)
		}
	}

	// Search with prefix "prod/"
	entries, err := v.SearchSecrets("prod/", "")
	if err != nil {
		t.Fatalf("SearchSecrets(prod/, '') failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for prefix 'prod/', got %d", len(entries))
	}
	if entries[0].Name != "prod/api/token" || entries[1].Name != "prod/db/pass" {
		t.Fatalf("unexpected search results order/content: %+v", entries)
	}

	// Search with substring "token" (case-insensitive)
	entries, err = v.SearchSecrets("", "token")
	if err != nil {
		t.Fatalf("SearchSecrets('', 'token') failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for substring 'token', got %d", len(entries))
	}
	// "common/TOKEN" and "prod/api/token"
	if entries[0].Name != "common/TOKEN" || entries[1].Name != "prod/api/token" {
		t.Fatalf("unexpected substring search results: %+v", entries)
	}

	// Both empty returns all secrets
	entries, err = v.SearchSecrets("", "")
	if err != nil {
		t.Fatalf("SearchSecrets('', '') failed: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries for empty filters, got %d", len(entries))
	}

	// Verify expired secret in search
	if err := v.PutWithTTL("temp/token", "val", 1*time.Minute); err != nil {
		t.Fatalf("PutWithTTL failed: %v", err)
	}
	fixedTime = fixedTime.Add(2 * time.Minute)
	entries, err = v.SearchSecrets("temp/", "")
	if err != nil {
		t.Fatalf("SearchSecrets temp/ failed: %v", err)
	}
	if len(entries) != 1 || !entries[0].Expired {
		t.Fatalf("expected 1 expired entry for temp/, got %+v", entries)
	}
}

func TestVault_AuditLogFiltered(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "audit_vault.json")

	t0 := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	curr := t0
	clock := func() time.Time { return curr }

	v, err := NewWithClock(vaultPath, clock)
	if err != nil {
		t.Fatalf("NewWithClock failed: %v", err)
	}
	if err := v.Initialize("master-password-12345"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	curr = t0.Add(1 * time.Minute)
	_ = v.Put("key1", "val1")

	curr = t0.Add(2 * time.Minute)
	_, _ = v.Resolve("missing_key") // records failure

	curr = t0.Add(3 * time.Minute)
	_ = v.Delete("key1")

	// Filter by Action "put"
	putEvts := v.AuditLogFiltered(AuditFilter{Action: "put"})
	if len(putEvts) == 0 {
		t.Fatalf("expected at least 1 'put' audit event")
	}
	for _, e := range putEvts {
		if e.Action != "put" {
			t.Fatalf("expected Action='put', got %q", e.Action)
		}
	}

	// Filter by Status "failure"
	failEvts := v.AuditLogFiltered(AuditFilter{Status: "failure"})
	if len(failEvts) == 0 {
		t.Fatalf("expected at least 1 'failure' audit event")
	}
	for _, e := range failEvts {
		if e.Status != "failure" {
			t.Fatalf("expected Status='failure', got %q", e.Status)
		}
	}

	// Filter by Since
	sinceEvts := v.AuditLogFiltered(AuditFilter{Since: t0.Add(2 * time.Minute)})
	if len(sinceEvts) != 2 { // get missing_key (2m) and delete key1 (3m)
		t.Fatalf("expected 2 audit events since t0+2m, got %d", len(sinceEvts))
	}
}

func TestVault_BatchDelete(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "batch_del_vault.json")

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// BatchDelete locked vault
	if _, err := v.BatchDelete([]string{"key1"}); err != ErrLocked {
		t.Fatalf("BatchDelete locked expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("master-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Put secrets
	_ = v.Put("k1", "v1")
	_ = v.Put("k2", "v2")
	_ = v.Put("k3", "v3")

	// BatchDelete with mix of valid, invalid name, and missing
	res, err := v.BatchDelete([]string{"k1", "k3", "missing", "invalid..name"})
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	if len(res.Deleted) != 2 || res.Deleted[0] != "k1" || res.Deleted[1] != "k3" {
		t.Fatalf("unexpected deleted list: %+v", res.Deleted)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %+v", res.Errors)
	}

	// Check remaining secrets
	st := v.Status()
	if len(st.Secrets) != 1 || st.Secrets[0].Name != "k2" {
		t.Fatalf("expected only k2 remaining, got %+v", st.Secrets)
	}

	// Durability check: reopen vault
	v2, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New v2 failed: %v", err)
	}
	if err := v2.Unlock("master-password"); err != nil {
		t.Fatalf("Unlock v2 failed: %v", err)
	}
	st2 := v2.Status()
	if len(st2.Secrets) != 1 || st2.Secrets[0].Name != "k2" {
		t.Fatalf("expected k2 in reopened vault, got %+v", st2.Secrets)
	}
}

func TestHTTP_BatchSearchAudit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "http_batch_vault.json")

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpass123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	_ = v.Put("app/prod/db", "secret1")
	_ = v.Put("app/prod/redis", "secret2")
	_ = v.Put("app/dev/db", "secret3")

	handler := HTTP{Vault: v}.Handler()

	// GET /secrets?prefix=app/prod/
	req := httptest.NewRequest(http.MethodGet, "/secrets?prefix=app/prod/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /secrets?prefix=... returned code %d", rec.Code)
	}
	var searchResult struct {
		Secrets []SecretEntry `json:"secrets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &searchResult); err != nil {
		t.Fatalf("Unmarshal search response failed: %v", err)
	}
	if len(searchResult.Secrets) != 2 {
		t.Fatalf("expected 2 entries for prefix search, got %d", len(searchResult.Secrets))
	}

	// GET /audit?action=put
	req = httptest.NewRequest(http.MethodGet, "/audit?action=put", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /audit?action=put returned code %d", rec.Code)
	}
	var auditEvts []AuditEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &auditEvts); err != nil {
		t.Fatalf("Unmarshal audit response failed: %v", err)
	}
	if len(auditEvts) < 3 {
		t.Fatalf("expected at least 3 put audit events, got %d", len(auditEvts))
	}

	// POST /secrets/batch-delete
	body, _ := json.Marshal(map[string]any{
		"names": []string{"app/prod/db", "app/prod/redis"},
	})
	req = httptest.NewRequest(http.MethodPost, "/secrets/batch-delete", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /secrets/batch-delete returned code %d: %s", rec.Code, rec.Body.String())
	}
	var delRes BatchDeleteResult
	if err := json.Unmarshal(rec.Body.Bytes(), &delRes); err != nil {
		t.Fatalf("Unmarshal batch delete response failed: %v", err)
	}
	if len(delRes.Deleted) != 2 {
		t.Fatalf("expected 2 deleted secrets, got %+v", delRes)
	}
}
