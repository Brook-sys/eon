package secretvault

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newUnlockedVault(t *testing.T) *Vault {
	t.Helper()
	dir := t.TempDir()
	v, err := New(filepath.Join(dir, "vault.json"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	return v
}

func TestVaultCopySecret(t *testing.T) {
	v := newUnlockedVault(t)

	// Locked vault must return ErrLocked.
	locked, err := New(filepath.Join(t.TempDir(), "locked.json"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := locked.CopySecret("a", "b"); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Put("src", "value-1"); err != nil {
		t.Fatalf("Put src failed: %v", err)
	}

	// Copy preserves the value; destination gets fresh lifecycle timestamps.
	before := time.Now().Add(-time.Second)
	if err := v.CopySecret("src", "dst"); err != nil {
		t.Fatalf("CopySecret failed: %v", err)
	}
	got, err := v.Resolve("dst")
	if err != nil {
		t.Fatalf("Resolve dst failed: %v", err)
	}
	if got != "value-1" {
		t.Fatalf("expected copied value value-1, got %q", got)
	}
	srcMeta, err := v.SecretMetadata("src")
	if err != nil {
		t.Fatalf("SecretMetadata src failed: %v", err)
	}
	dstMeta, err := v.SecretMetadata("dst")
	if err != nil {
		t.Fatalf("SecretMetadata dst failed: %v", err)
	}
	if !srcMeta.CreatedAt.Equal(dstMeta.CreatedAt) {
		t.Log("dst has its own lifecycle timestamps (expected fresh)")
	}
	if dstMeta.CreatedAt.Before(before) {
		t.Fatalf("dst CreatedAt %v should be fresh (>= %v)", dstMeta.CreatedAt, before)
	}

	// Source survives the copy.
	if got, err := v.Resolve("src"); err != nil || got != "value-1" {
		t.Fatalf("src lost after copy: value=%q err=%v", got, err)
	}

	// Copy of a missing source returns os.ErrNotExist and does not clobber dst.
	if err := v.CopySecret("missing", "dst"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
	if got, err := v.Resolve("dst"); err != nil || got != "value-1" {
		t.Fatalf("dst clobbered by failed copy: value=%q err=%v", got, err)
	}

	// Invalid names are rejected on both sides.
	if err := v.CopySecret("bad//name", "x"); err == nil {
		t.Fatal("expected error for invalid source name")
	}
	if err := v.CopySecret("src", "bad//name"); err == nil {
		t.Fatal("expected error for invalid destination name")
	}

	// TTL carries over: copying a secret with an expiration preserves ExpiresAt.
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := v.PutWithExpiry("ttl-src", "ttl-val", exp); err != nil {
		t.Fatalf("PutWithExpiry failed: %v", err)
	}
	if err := v.CopySecret("ttl-src", "ttl-dst"); err != nil {
		t.Fatalf("CopySecret ttl failed: %v", err)
	}
	ttlDstMeta, err := v.SecretMetadata("ttl-dst")
	if err != nil {
		t.Fatalf("SecretMetadata ttl-dst failed: %v", err)
	}
	if ttlDstMeta.ExpiresAt.IsZero() || !ttlDstMeta.ExpiresAt.Equal(exp) {
		t.Fatalf("expected ExpiresAt %v, got %v", exp, ttlDstMeta.ExpiresAt)
	}

	// Copy overwrites an existing destination (upsert semantics like Put).
	if err := v.CopySecret("src", "ttl-dst"); err != nil {
		t.Fatalf("CopySecret overwrite failed: %v", err)
	}
	if got, err := v.Resolve("ttl-dst"); err != nil || got != "value-1" {
		t.Fatalf("overwrite did not take: value=%q err=%v", got, err)
	}
}

func TestVaultRenameSecret(t *testing.T) {
	v := newUnlockedVault(t)

	// Locked vault must return ErrLocked.
	locked, err := New(filepath.Join(t.TempDir(), "locked.json"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := locked.RenameSecret("a", "b"); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Put("old", "secret-value"); err != nil {
		t.Fatalf("Put old failed: %v", err)
	}
	origMeta, err := v.SecretMetadata("old")
	if err != nil {
		t.Fatalf("SecretMetadata old failed: %v", err)
	}

	// Rename moves the value, keeps CreatedAt, advances UpdatedAt.
	time.Sleep(5 * time.Millisecond)
	if err := v.RenameSecret("old", "new"); err != nil {
		t.Fatalf("RenameSecret failed: %v", err)
	}
	if _, err := v.Resolve("old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old name should be gone after rename, err=%v", err)
	}
	got, err := v.Resolve("new")
	if err != nil {
		t.Fatalf("Resolve new failed: %v", err)
	}
	if got != "secret-value" {
		t.Fatalf("expected secret-value, got %q", got)
	}
	newMeta, err := v.SecretMetadata("new")
	if err != nil {
		t.Fatalf("SecretMetadata new failed: %v", err)
	}
	if !newMeta.CreatedAt.Equal(origMeta.CreatedAt) {
		t.Fatalf("CreatedAt must be preserved across rename: orig=%v new=%v", origMeta.CreatedAt, newMeta.CreatedAt)
	}
	if !newMeta.UpdatedAt.After(origMeta.CreatedAt) {
		t.Fatalf("UpdatedAt %v should advance past CreatedAt %v", newMeta.UpdatedAt, origMeta.CreatedAt)
	}

	// Rename of a missing source returns os.ErrNotExist.
	if err := v.RenameSecret("missing", "whatever"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	// Invalid names rejected on both sides.
	if err := v.RenameSecret("bad//name", "x"); err == nil {
		t.Fatal("expected error for invalid source name")
	}
	if err := v.RenameSecret("new", "bad//name"); err == nil {
		t.Fatal("expected error for invalid destination name")
	}

	// TTL carries over on rename.
	exp := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if err := v.PutWithExpiry("ttl-old", "ttl-val", exp); err != nil {
		t.Fatalf("PutWithExpiry failed: %v", err)
	}
	if err := v.RenameSecret("ttl-old", "ttl-new"); err != nil {
		t.Fatalf("RenameSecret ttl failed: %v", err)
	}
	ttlNewMeta, err := v.SecretMetadata("ttl-new")
	if err != nil {
		t.Fatalf("SecretMetadata ttl-new failed: %v", err)
	}
	if ttlNewMeta.ExpiresAt.IsZero() || !ttlNewMeta.ExpiresAt.Equal(exp) {
		t.Fatalf("expected ExpiresAt %v preserved, got %v", exp, ttlNewMeta.ExpiresAt)
	}

	// Rename over an existing destination overwrites it.
	if err := v.RenameSecret("new", "ttl-new"); err != nil {
		t.Fatalf("RenameSecret overwrite failed: %v", err)
	}
	if got, err := v.Resolve("ttl-new"); err != nil || got != "secret-value" {
		t.Fatalf("overwrite did not take: value=%q err=%v", got, err)
	}
	if _, err := v.Resolve("new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source name should be gone after rename-overwrite, err=%v", err)
	}

	// Self-rename is a no-op metadata refresh, not an error.
	if err := v.Put("self", "self-val"); err != nil {
		t.Fatalf("Put self failed: %v", err)
	}
	if err := v.RenameSecret("self", "self"); err != nil {
		t.Fatalf("self-rename failed: %v", err)
	}
	if got, err := v.Resolve("self"); err != nil || got != "self-val" {
		t.Fatalf("self-rename lost value: value=%q err=%v", got, err)
	}
}

func TestVaultCopySecretPersistenceAndAudit(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.json")
	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("a", "va"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := v.CopySecret("a", "b"); err != nil {
		t.Fatalf("CopySecret failed: %v", err)
	}
	if err := v.RenameSecret("b", "c"); err != nil {
		t.Fatalf("RenameSecret failed: %v", err)
	}

	// Reopen from disk: both copy and rename must survive.
	v2, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New (reopen) failed: %v", err)
	}
	if err := v2.Unlock("super-secret-password"); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
	if got, err := v2.Resolve("a"); err != nil || got != "va" {
		t.Fatalf("a lost: value=%q err=%v", got, err)
	}
	if got, err := v2.Resolve("c"); err != nil || got != "va" {
		t.Fatalf("c lost: value=%q err=%v", got, err)
	}
	if _, err := v2.Resolve("b"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("b should not exist after rename, err=%v", err)
	}

	// Audit trail is in-memory only: check copy and rename events on the
	// original (still open) vault, not on the reopened one.
	evs, err := v.SecretHistory("c")
	if err != nil {
		t.Fatalf("SecretHistory c failed: %v", err)
	}
	var sawRename bool
	for _, e := range evs {
		if e.Action == "rename" && e.Status == "success" {
			sawRename = true
		}
	}
	if !sawRename {
		t.Fatal("expected rename success audit event for c")
	}
	evsB, bErr := v.SecretHistory("b")
	if bErr != nil {
		t.Fatalf("SecretHistory b failed (audit recorded under destination b): %v", bErr)
	}
	var sawCopy bool
	for _, e := range evsB {
		if e.Action == "copy" && e.Status == "success" {
			sawCopy = true
		}
	}
	if !sawCopy {
		t.Fatal("expected copy success audit event recorded under destination b")
	}
}

func TestHTTPCopyAndRename(t *testing.T) {
	dir := t.TempDir()
	v, err := New(filepath.Join(dir, "vault.json"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("super-secret-password"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("k1", "v1"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	srv := httptest.NewServer(HTTP{Vault: v}.Handler())
	defer srv.Close()

	post := func(path, body string) *http.Response {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s failed: %v", path, err)
		}
		return resp
	}

	// POST /secrets/{name}/copy -> 201 Created, value available at destination.
	resp := post("/secrets/k1/copy", `{"destination":"k2"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("copy expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	if got, err := v.Resolve("k2"); err != nil || got != "v1" {
		t.Fatalf("k2 not copied: value=%q err=%v", got, err)
	}

	// Copy of a missing source -> 404.
	resp = post("/secrets/absent/copy", `{"destination":"k3"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("copy missing expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Empty destination -> 400.
	resp = post("/secrets/k1/copy", `{"destination":""}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("copy empty destination expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// POST /secrets/{name}/rename -> 204 No Content, source gone.
	resp = post("/secrets/k2/rename", `{"destination":"k4"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("rename expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := v.Resolve("k2"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("k2 should be gone after rename, err=%v", err)
	}
	if got, err := v.Resolve("k4"); err != nil || got != "v1" {
		t.Fatalf("k4 missing after rename: value=%q err=%v", got, err)
	}

	// Rename of missing source -> 404.
	resp = post("/secrets/absent/rename", `{"destination":"x"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("rename missing expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Locked vault -> 423 on both endpoints.
	v.Lock()
	resp = post("/secrets/k1/copy", `{"destination":"k5"}`)
	if resp.StatusCode != http.StatusLocked {
		t.Fatalf("copy locked expected 423, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = post("/secrets/k1/rename", `{"destination":"k5"}`)
	if resp.StatusCode != http.StatusLocked {
		t.Fatalf("rename locked expected 423, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
