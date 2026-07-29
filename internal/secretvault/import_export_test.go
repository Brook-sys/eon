package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	vaultPath1 := filepath.Join(dir, "vault1.vault")
	vaultPath2 := filepath.Join(dir, "vault2.vault")
	backupPath := filepath.Join(dir, "backup.env")

	const pass1 = "correct horse battery staple"
	const pass2 = "another valid password 123"
	const backupPass = "backup password string 123"

	// Create and initialize Vault 1 with secrets
	v1, err := New(vaultPath1)
	if err != nil {
		t.Fatalf("New vault1: %v", err)
	}
	if err := v1.Initialize(pass1); err != nil {
		t.Fatalf("Initialize vault1: %v", err)
	}
	if err := v1.Put("groq/api_key", "gsk-12345"); err != nil {
		t.Fatalf("Put secret 1: %v", err)
	}
	if err := v1.Put("nvidia/api_key", "nvapi-67890"); err != nil {
		t.Fatalf("Put secret 2: %v", err)
	}

	// Export Vault 1 to backup file
	if err := v1.Export(backupPath, backupPass); err != nil {
		t.Fatalf("Export vault1: %v", err)
	}

	// Verify backup file exists and is permissions 0600
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("Stat backup: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("backup mode = %o, expected 0600", info.Mode().Perm())
	}

	// Create and initialize Vault 2
	v2, err := New(vaultPath2)
	if err != nil {
		t.Fatalf("New vault2: %v", err)
	}
	if err := v2.Initialize(pass2); err != nil {
		t.Fatalf("Initialize vault2: %v", err)
	}

	// Import into Vault 2
	if err := v2.Import(backupPath, backupPass); err != nil {
		t.Fatalf("Import vault2: %v", err)
	}

	// Verify secrets in Vault 2
	val1, err := v2.Resolve("groq/api_key")
	if err != nil || val1 != "gsk-12345" {
		t.Fatalf("Resolve groq/api_key = %q, %v; expected gsk-12345", val1, err)
	}
	val2, err := v2.Resolve("nvidia/api_key")
	if err != nil || val2 != "nvapi-67890" {
		t.Fatalf("Resolve nvidia/api_key = %q, %v; expected nvapi-67890", val2, err)
	}
}

func TestImportValidationErrors(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.vault")
	backupPath := filepath.Join(dir, "backup.env")
	badPath := filepath.Join(dir, "nonexistent.env")

	const pass = "correct horse battery staple"
	const backupPass = "backup password string 123"

	v, err := New(vaultPath)
	if err != nil {
		t.Fatalf("New vault: %v", err)
	}
	if err := v.Initialize(pass); err != nil {
		t.Fatalf("Initialize vault: %v", err)
	}
	if err := v.Put("test/key", "val"); err != nil {
		t.Fatalf("Put secret: %v", err)
	}

	// Test Export while locked
	v.Lock()
	if err := v.Export(backupPath, backupPass); err != ErrLocked {
		t.Errorf("Export locked = %v, expected ErrLocked", err)
	}

	// Unlock and export
	if err := v.Unlock(pass); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := v.Export(backupPath, backupPass); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Import non-existent file
	if err := v.Import(badPath, backupPass); !os.IsNotExist(err) {
		t.Errorf("Import nonexistent = %v, expected os.ErrNotExist", err)
	}

	// Import with invalid backup password
	v2, _ := New(filepath.Join(dir, "vault2.vault"))
	_ = v2.Initialize(pass)
	if err := v2.Import(backupPath, "wrong backup password"); err != ErrInvalidPassword {
		t.Errorf("Import wrong password = %v, expected ErrInvalidPassword", err)
	}

	// Import with conflict
	v3, _ := New(filepath.Join(dir, "vault3.vault"))
	_ = v3.Initialize(pass)
	_ = v3.Put("test/key", "conflicting_value")
	if err := v3.Import(backupPath, backupPass); err != ErrImportConflict {
		t.Errorf("Import conflict = %v, expected ErrImportConflict", err)
	}

	// Import corrupted JSON file
	corruptPath := filepath.Join(dir, "corrupt.env")
	if err := os.WriteFile(corruptPath, []byte("invalid json data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := v2.Import(corruptPath, backupPass); err != ErrInvalidBackupFormat {
		t.Errorf("Import corrupt = %v, expected ErrInvalidBackupFormat", err)
	}
}

func TestHTTPExportAndImport(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.vault")
	backupPath := filepath.Join(dir, "backup.env")
	targetPath := filepath.Join(dir, "target.vault")

	const pass = "correct horse battery staple"
	const backupPass = "backup password string 123"

	v, _ := New(vaultPath)
	_ = v.Initialize(pass)
	_ = v.Put("http/key", "http-secret")

	ts := httptest.NewServer(HTTP{Vault: v}.Handler())
	defer ts.Close()

	// HTTP Export
	body, _ := json.Marshal(map[string]string{
		"backup_path":     backupPath,
		"backup_password": backupPass,
	})
	resp, err := http.Post(ts.URL+"/export", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /export: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /export status = %d, expected 200", resp.StatusCode)
	}
	resp.Body.Close()

	// HTTP Import into target vault
	vTarget, _ := New(targetPath)
	_ = vTarget.Initialize(pass)
	tsTarget := httptest.NewServer(HTTP{Vault: vTarget}.Handler())
	defer tsTarget.Close()

	resp, err = http.Post(tsTarget.URL+"/import", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /import: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /import status = %d, expected 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify imported secret
	val, err := vTarget.Resolve("http/key")
	if err != nil || val != "http-secret" {
		t.Fatalf("Target resolve http/key = %q, %v; expected http-secret", val, err)
	}

	// Conflict via HTTP
	resp, err = http.Post(tsTarget.URL+"/import", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /import conflict: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST /import conflict status = %d, expected 409", resp.StatusCode)
	}
	resp.Body.Close()
}
