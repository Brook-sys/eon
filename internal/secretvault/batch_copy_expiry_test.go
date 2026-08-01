package secretvault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBatchCopyHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.bin")
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	v, err := NewWithClock(path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWithClock: %v", err)
	}
	if err := v.Initialize("correct horse battery"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := v.PutWithTTL("svc/primary", "secret-one", 2*time.Hour); err != nil {
		t.Fatalf("PutWithTTL: %v", err)
	}
	if err := v.Put("svc/plain", "secret-two"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	missing := filepath.Join(dir, "vault.bin")
	_ = missing

	res, err := v.BatchCopy([]BatchCopyItem{
		{Source: "svc/primary", Destination: "svc/primary-backup"},
		{Source: "svc/plain", Destination: "svc/plain-backup"},
		{Source: "svc/does-not-exist", Destination: "svc/nope"},
		{Source: "bad name\x00", Destination: "svc/bad"},
		{Source: "svc/primary", Destination: "svc/primary"},        // src==dst
		{Source: "svc/primary", Destination: "svc/primary-backup"}, // duplicate dst
	})
	if err != nil {
		t.Fatalf("BatchCopy: %v", err)
	}
	if len(res.Copied) != 2 || res.Copied[0] != "svc/plain-backup" || res.Copied[1] != "svc/primary-backup" {
		t.Fatalf("Copied = %v, want [svc/plain-backup svc/primary-backup]", res.Copied)
	}
	if len(res.Errors) != 4 {
		t.Fatalf("Errors = %v, want 4 entries", res.Errors)
	}
	joined := strings.Join(res.Errors, "\n")
	for _, want := range []string{"not found", "invalid source name", "source equals destination", "duplicate destination in batch"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Errors missing %q: %v", want, res.Errors)
		}
	}

	// Expiration copied verbatim, fresh timestamps.
	meta, err := v.SecretMetadata("svc/primary-backup")
	if err != nil {
		t.Fatalf("SecretMetadata: %v", err)
	}
	wantExpiry := now.Add(2 * time.Hour)
	if !meta.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("ExpiresAt = %v, want %v", meta.ExpiresAt, wantExpiry)
	}
	if !meta.CreatedAt.Equal(now) || !meta.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v/%v, want %v", meta.CreatedAt, meta.UpdatedAt, now)
	}
	val, err := v.Resolve("svc/primary-backup")
	if err != nil || val != "secret-one" {
		t.Fatalf("Resolve = %q, %v", val, err)
	}

	// Durable across reopen.
	v2, err := NewWithClock(path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("reopen NewWithClock: %v", err)
	}
	if err := v2.Unlock("correct horse battery"); err != nil {
		t.Fatalf("reopen Unlock: %v", err)
	}
	val2, err := v2.Resolve("svc/primary-backup")
	if err != nil || val2 != "secret-one" {
		t.Fatalf("reopen Resolve = %q, %v", val2, err)
	}
	val3, err := v2.Resolve("svc/plain-backup")
	if err != nil || val3 != "secret-two" {
		t.Fatalf("reopen Resolve (no-expiry copy) = %q, %v", val3, err)
	}
	v2.Close()
	v.Close()
}

func TestBatchCopyLockedAndEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.bin")
	v, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.Initialize("correct horse battery"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	v.Lock()
	if _, err := v.BatchCopy([]BatchCopyItem{{Source: "a", Destination: "b"}}); err != ErrLocked {
		t.Fatalf("locked BatchCopy = %v, want ErrLocked", err)
	}
	if _, err := v.ExpiringSoon(time.Hour); err != ErrLocked {
		t.Fatalf("locked ExpiringSoon = %v, want ErrLocked", err)
	}
	if err := v.Unlock("correct horse battery"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	res, err := v.BatchCopy(nil)
	if err != nil {
		t.Fatalf("empty BatchCopy: %v", err)
	}
	if len(res.Copied) != 0 || len(res.Errors) != 0 {
		t.Fatalf("empty BatchCopy = %+v, want no copies/errors", res)
	}
	v.Close()
}

func TestBatchCopyFailedSaveKeepsInMemoryState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.bin")
	v, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.Initialize("correct horse battery"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := v.Put("keep/me", "original"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Break the vault file so the next save/reload cycle fails: replace it
	// with an unwritable location by removing write permission on the dir.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skipf("chmod unsupported: %v", err)
	}
	defer os.Chmod(dir, 0o755)
	_, err = v.BatchCopy([]BatchCopyItem{{Source: "keep/me", Destination: "keep/copy"}})
	if err == nil {
		t.Fatalf("BatchCopy on read-only dir succeeded, want save error")
	}
	// In-memory state must not contain the failed copy.
	if ok, _ := v.Exists("keep/copy"); ok {
		t.Fatalf("keep/copy visible in memory after failed save; state diverged")
	}
	os.Chmod(dir, 0o755)
	v2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := v2.Unlock("correct horse battery"); err != nil {
		t.Fatalf("reopen Unlock: %v", err)
	}
	if ok, _ := v2.Exists("keep/copy"); ok {
		t.Fatalf("keep/copy persisted despite failed save")
	}
	v2.Close()
	v.Close()
}

func TestExpiringSoonFilteringAndOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.bin")
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	v, err := NewWithClock(path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWithClock: %v", err)
	}
	if err := v.Initialize("correct horse battery"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	seed := []struct {
		name string
		ttl  time.Duration
	}{
		{"k/in-1h", time.Hour},
		{"k/in-30m", 30 * time.Minute},
		{"k/in-2h", 2 * time.Hour},
		{"z/same-time-1h", time.Hour}, // tie with k/in-1h, name must order
	}
	for _, s := range seed {
		if err := v.PutWithTTL(s.name, "v", s.ttl); err != nil {
			t.Fatalf("PutWithTTL %s: %v", s.name, err)
		}
	}
	// Already-elapsed record via absolute expiry in the past: expireLocked
	// treats it as inactive for resolution/listing but the record persists
	// until PurgeExpired; ExpiringSoon is forward-only and must exclude it.
	if err := v.PutWithExpiry("k/expired", "v", now.Add(-time.Minute)); err != nil {
		t.Fatalf("PutWithExpiry k/expired: %v", err)
	}
	if err := v.Put("k/no-expiry", "v"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	items, err := v.ExpiringSoon(90 * time.Minute)
	if err != nil {
		t.Fatalf("ExpiringSoon: %v", err)
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.Name)
		if it.ExpiresAt.IsZero() || it.Remaining == "" {
			t.Fatalf("item %+v missing expiry info", it)
		}
	}
	want := []string{"k/in-30m", "k/in-1h", "z/same-time-1h"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ExpiringSoon order = %v, want %v", got, want)
	}

	// Window covering everything still excludes no-expiry entries.
	items2, err := v.ExpiringSoon(48 * time.Hour)
	if err != nil {
		t.Fatalf("ExpiringSoon large: %v", err)
	}
	for _, it := range items2 {
		if it.Name == "k/no-expiry" {
			t.Fatalf("no-expiry secret leaked into ExpiringSoon")
		}
	}
	if len(items2) != 4 {
		t.Fatalf("ExpiringSoon 48h count = %d, want 4 (in-30m, in-1h, same-time-1h, in-2h)", len(items2))
	}

	// Non-positive window → empty.
	items3, err := v.ExpiringSoon(0)
	if err != nil || len(items3) != 0 {
		t.Fatalf("ExpiringSoon(0) = %v, %v; want empty", items3, err)
	}
	v.Close()
}

func TestBatchCopyAndExpiringSoonHTTP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.bin")
	v, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := v.Initialize("correct horse battery"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := v.PutWithTTL("api/key", "k", 30*time.Minute); err != nil {
		t.Fatalf("PutWithTTL: %v", err)
	}
	h := HTTP{Vault: v}.Handler()
	loopback := func(req *http.Request) *http.Request {
		req.RemoteAddr = "127.0.0.1:12345"
		return req
	}

	// Route registration sanity: the expiring-soon route must be reachable
	// before the wildcard GET /secrets/{name} handler shadows it.
	req := loopback(httptest.NewRequest(http.MethodGet, "/secrets/expiring-soon?window=1h", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET expiring-soon code = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Window string             `json:"window"`
		Items  []ExpiringSoonItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if payload.Window != "1h0m0s" || len(payload.Items) != 1 || payload.Items[0].Name != "api/key" {
		t.Fatalf("payload = %+v, want one api/key item under 1h window", payload)
	}

	// Batch copy round-trip through HTTP.
	body := `{"items":[{"source":"api/key","destination":"api/key-copy"},{"source":"missing","destination":"x"}]}`
	req2 := loopback(httptest.NewRequest(http.MethodPost, "/secrets/batch-copy", strings.NewReader(body)))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST batch-copy code = %d body=%s", rec2.Code, rec2.Body.String())
	}
	var cp BatchCopyResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &cp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(cp.Copied) != 1 || cp.Copied[0] != "api/key-copy" || len(cp.Errors) != 1 {
		t.Fatalf("batch-copy result = %+v", cp)
	}

	// Invalid window → 400 (ErrInvalidTTL mapping).
	req3 := loopback(httptest.NewRequest(http.MethodGet, "/secrets/expiring-soon?window=nope", nil))
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("invalid window code = %d, want 400", rec3.Code)
	}

	// Empty batch → 400.
	req4 := loopback(httptest.NewRequest(http.MethodPost, "/secrets/batch-copy", strings.NewReader(`{"items":[]}`)))
	rec4 := httptest.NewRecorder()
	h.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusBadRequest {
		t.Fatalf("empty batch-copy code = %d, want 400", rec4.Code)
	}
	v.Close()
}
