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

	// Search with prefix and substring combined
	entries, err = v.SearchSecrets("prod/", "token")
	if err != nil {
		t.Fatalf("SearchSecrets(prod/, 'token') failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "prod/api/token" {
		t.Fatalf("expected 1 entry 'prod/api/token' for prefix+substring, got %+v", entries)
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

func TestVault_AuditSummaryAndHTTP(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "summary_vault.json")

	t0 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
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
	if err := v.Put("alpha", "value-alpha"); err != nil {
		t.Fatalf("Put alpha failed: %v", err)
	}
	curr = t0.Add(2 * time.Minute)
	if err := v.Put("beta", "value-beta"); err != nil {
		t.Fatalf("Put beta failed: %v", err)
	}
	curr = t0.Add(3 * time.Minute)
	if _, err := v.Resolve("missing"); err == nil {
		t.Fatalf("expected Resolve missing to fail")
	}
	curr = t0.Add(4 * time.Minute)
	if err := v.Delete("alpha"); err != nil {
		t.Fatalf("Delete alpha failed: %v", err)
	}

	// Unfiltered summary
	sum := v.AuditSummary(AuditFilter{})
	if sum.MatchedEvents != sum.TotalEvents {
		t.Fatalf("expected MatchedEvents=%d, got %d", sum.TotalEvents, sum.MatchedEvents)
	}
	if sum.TotalEvents < 5 { // initialize + 2 puts + 1 get(fail) + 1 delete
		t.Fatalf("expected >=5 total events, got %d", sum.TotalEvents)
	}
	if sum.Actions["put"] != 2 {
		t.Fatalf("expected 2 put events, got %d", sum.Actions["put"])
	}
	if sum.Statuses["failure"] < 1 {
		t.Fatalf("expected >=1 failure status, got %d", sum.Statuses["failure"])
	}
	if sum.DistinctSecrets < 3 { // alpha, beta, missing (missing may or may not be recorded)
		t.Fatalf("expected >=3 distinct secrets, got %d", sum.DistinctSecrets)
	}
	if sum.FirstEventAt.After(sum.LastEventAt) {
		t.Fatalf("FirstEventAt %v after LastEventAt %v", sum.FirstEventAt, sum.LastEventAt)
	}
	if !sum.FirstEventAt.Equal(t0) {
		t.Fatalf("expected FirstEventAt=%v, got %v", t0, sum.FirstEventAt)
	}

	// Filtered by action=put
	putSum := v.AuditSummary(AuditFilter{Action: "put"})
	if putSum.MatchedEvents != 2 {
		t.Fatalf("expected 2 matched put events, got %d", putSum.MatchedEvents)
	}
	if len(putSum.Actions) != 1 || putSum.Actions["put"] != 2 {
		t.Fatalf("expected only put action, got %+v", putSum.Actions)
	}
	if putSum.DistinctSecrets != 2 {
		t.Fatalf("expected 2 distinct secrets in puts, got %d", putSum.DistinctSecrets)
	}

	// Filtered by status=failure
	failSum := v.AuditSummary(AuditFilter{Status: "failure"})
	if failSum.MatchedEvents < 1 {
		t.Fatalf("expected >=1 matched failure events, got %d", failSum.MatchedEvents)
	}
	if len(failSum.Statuses) != 1 || failSum.Statuses["failure"] == 0 {
		t.Fatalf("expected only failure status, got %+v", failSum.Statuses)
	}

	// Time window filter: only t0+1m..t0+2m (both puts)
	winSum := v.AuditSummary(AuditFilter{Since: t0.Add(1 * time.Minute), Until: t0.Add(2 * time.Minute)})
	if winSum.MatchedEvents != 2 {
		t.Fatalf("expected 2 matched events in window, got %d", winSum.MatchedEvents)
	}

	// Limit field must be ignored by aggregation
	limSum := v.AuditSummary(AuditFilter{Limit: 1})
	if limSum.MatchedEvents != sum.MatchedEvents {
		t.Fatalf("Limit must be ignored: expected %d, got %d", sum.MatchedEvents, limSum.MatchedEvents)
	}

	// Summary must not leak secret values
	raw, _ := json.Marshal(sum)
	if bytes.Contains(raw, []byte("value-alpha")) || bytes.Contains(raw, []byte("value-beta")) || bytes.Contains(raw, []byte("master-password")) {
		t.Fatalf("AuditSummary leaked sensitive data: %s", raw)
	}

	// HTTP endpoint
	srv := httptest.NewServer(HTTP{Vault: v}.Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/audit/summary")
	if err != nil {
		t.Fatalf("GET /audit/summary failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var httpSum AuditSummary
	if err := json.NewDecoder(res.Body).Decode(&httpSum); err != nil {
		t.Fatalf("decode summary failed: %v", err)
	}
	if httpSum.TotalEvents != sum.TotalEvents || httpSum.MatchedEvents != sum.MatchedEvents {
		t.Fatalf("HTTP summary mismatch: %+v vs %+v", httpSum, sum)
	}

	// HTTP with filters
	res2, err := http.Get(srv.URL + "/audit/summary?action=put&since=" + t0.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("GET /audit/summary filtered failed: %v", err)
	}
	defer res2.Body.Close()
	var filtSum AuditSummary
	if err := json.NewDecoder(res2.Body).Decode(&filtSum); err != nil {
		t.Fatalf("decode filtered summary failed: %v", err)
	}
	if filtSum.MatchedEvents != 2 {
		t.Fatalf("expected 2 matched via HTTP, got %d", filtSum.MatchedEvents)
	}

	// Bad since/until should 400
	for _, bad := range []string{"since=not-a-date", "until=oops"} {
		badRes, err := http.Get(srv.URL + "/audit/summary?" + bad)
		if err != nil {
			t.Fatalf("GET with %s failed: %v", bad, err)
		}
		if badRes.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", bad, badRes.StatusCode)
		}
		badRes.Body.Close()
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
