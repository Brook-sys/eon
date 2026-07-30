package secretvault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestVaultSecretMetadata(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// SecretMetadata on locked vault returns ErrLocked.
	if _, err := v.SecretMetadata("key1"); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Non-existent secret returns os.ErrNotExist.
	if _, err := v.SecretMetadata("missing"); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	// Invalid name returns ErrInvalidSecretName.
	if _, err := v.SecretMetadata("../bad"); err != ErrInvalidSecretName {
		t.Fatalf("expected ErrInvalidSecretName, got %v", err)
	}

	// Put a secret and check metadata.
	if err := v.Put("db_pass", "secret123"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	meta, err := v.SecretMetadata("db_pass")
	if err != nil {
		t.Fatalf("SecretMetadata failed: %v", err)
	}
	if meta.Name != "db_pass" {
		t.Fatalf("expected name db_pass, got %s", meta.Name)
	}
	if meta.Expired {
		t.Fatalf("expected Expired to be false")
	}
	if meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() {
		t.Fatalf("expected non-zero CreatedAt/UpdatedAt")
	}

	// Secret with TTL.
	if err := v.PutWithTTL("temp_token", "tok", 10*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	meta, err = v.SecretMetadata("temp_token")
	if err != nil {
		t.Fatalf("SecretMetadata on expired secret failed: %v", err)
	}
	if !meta.Expired {
		t.Fatalf("expected Expired to be true for expired secret")
	}
}

func TestVaultStats(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Stats on uninitialized vault.
	st := v.Stats()
	if st.Initialized {
		t.Fatalf("expected Initialized=false")
	}
	if !st.Locked {
		t.Fatalf("expected Locked=true")
	}

	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Stats on initialized, unlocked vault.
	st = v.Stats()
	if !st.Initialized {
		t.Fatalf("expected Initialized=true")
	}
	if st.Locked {
		t.Fatalf("expected Locked=false")
	}
	if st.TotalSecrets != 0 {
		t.Fatalf("expected TotalSecrets=0, got %d", st.TotalSecrets)
	}

	// Add secrets: 1 normal, 1 expiring soon (30m), 1 expired.
	if err := v.Put("normal_key", "val1"); err != nil {
		t.Fatalf("Put normal_key failed: %v", err)
	}
	if err := v.PutWithTTL("soon_key", "val2", 30*time.Minute); err != nil {
		t.Fatalf("PutWithTTL soon_key failed: %v", err)
	}
	if err := v.PutWithTTL("expired_key", "val3", 10*time.Millisecond); err != nil {
		t.Fatalf("PutWithTTL expired_key failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	st = v.Stats()
	if st.TotalSecrets != 3 {
		t.Fatalf("expected TotalSecrets=3, got %d", st.TotalSecrets)
	}
	if st.ExpiredCount != 1 {
		t.Fatalf("expected ExpiredCount=1, got %d", st.ExpiredCount)
	}
	if st.ExpiringSoon != 1 {
		t.Fatalf("expected ExpiringSoon=1, got %d", st.ExpiringSoon)
	}
	if st.AuditEntries < 3 {
		t.Fatalf("expected AuditEntries >= 3, got %d", st.AuditEntries)
	}
}

func TestHTTPStatsAndMetadata(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vault.enc"
	v, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := v.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if err := v.Put("api_key", "secret-value"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	srv := httptest.NewServer(HTTP{Vault: v}.Handler())
	defer srv.Close()

	// GET /stats
	resp, err := http.Get(srv.URL + "/stats")
	if err != nil {
		t.Fatalf("GET /stats failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	var stats VaultStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("Decode stats failed: %v", err)
	}
	if stats.TotalSecrets != 1 {
		t.Fatalf("expected TotalSecrets=1, got %d", stats.TotalSecrets)
	}
	if !stats.Initialized || stats.Locked {
		t.Fatalf("unexpected state in stats: %+v", stats)
	}

	// GET /secrets/api_key/metadata
	resp, err = http.Get(srv.URL + "/secrets/api_key/metadata")
	if err != nil {
		t.Fatalf("GET metadata failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}

	var meta SecretEntry
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatalf("Decode metadata failed: %v", err)
	}
	if meta.Name != "api_key" {
		t.Fatalf("expected api_key, got %s", meta.Name)
	}

	// GET /secrets/nonexistent/metadata -> HTTP 404
	resp, err = http.Get(srv.URL + "/secrets/nonexistent/metadata")
	if err != nil {
		t.Fatalf("GET nonexistent metadata failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d", resp.StatusCode)
	}
}

func TestImportResultOverwritten(t *testing.T) {
	dir := t.TempDir()
	srcPath := dir + "/src.enc"
	dstPath := dir + "/dst.enc"
	backupPath := dir + "/backup.json"

	// Create source vault with secrets.
	src, err := New(srcPath)
	if err != nil {
		t.Fatalf("New src failed: %v", err)
	}
	if err := src.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Init src failed: %v", err)
	}
	if err := src.Put("key1", "val1"); err != nil {
		t.Fatalf("Put key1 failed: %v", err)
	}
	if err := src.Put("key2", "val2"); err != nil {
		t.Fatalf("Put key2 failed: %v", err)
	}
	if err := src.Export(backupPath, "backup-password-123"); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Create destination vault with key1 already present.
	dst, err := New(dstPath)
	if err != nil {
		t.Fatalf("New dst failed: %v", err)
	}
	if err := dst.Initialize("masterpassword123"); err != nil {
		t.Fatalf("Init dst failed: %v", err)
	}
	if err := dst.Put("key1", "old_val1"); err != nil {
		t.Fatalf("Put key1 in dst failed: %v", err)
	}

	// Import with Mode Overwrite.
	res, err := dst.ImportWithOptions(backupPath, "backup-password-123", ImportOptions{Mode: ImportModeOverwrite})
	if err != nil {
		t.Fatalf("ImportWithOptions failed: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("expected Total=2, got %d", res.Total)
	}
	if res.Imported != 2 {
		t.Fatalf("expected Imported=2, got %d", res.Imported)
	}
	if res.Overwritten != 1 {
		t.Fatalf("expected Overwritten=1, got %d", res.Overwritten)
	}
	if res.Skipped != 0 {
		t.Fatalf("expected Skipped=0, got %d", res.Skipped)
	}
}
