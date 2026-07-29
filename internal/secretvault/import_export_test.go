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

func TestImportWithOptionsModes(t *testing.T) {
	dir := t.TempDir()
	v1, _ := New(filepath.Join(dir, "v1.vault"))
	const pass = "correct horse battery staple"
	const backupPass = "backup password string 123"
	_ = v1.Initialize(pass)
	_ = v1.Put("existing/key", "original_value")
	_ = v1.Put("new/key", "imported_value")

	backupPath := filepath.Join(dir, "backup.env")
	if err := v1.Export(backupPath, backupPass); err != nil {
		t.Fatalf("Export v1: %v", err)
	}

	// Target vault with a conflicting key
	v2, _ := New(filepath.Join(dir, "v2.vault"))
	_ = v2.Initialize(pass)
	_ = v2.Put("existing/key", "current_value")

	// Invalid mode
	if err := v2.ImportWithOptions(backupPath, backupPass, ImportOptions{Mode: ImportMode(99)}); err != ErrInvalidImportMode {
		t.Errorf("ImportWithOptions invalid mode = %v, expected ErrInvalidImportMode", err)
	}

	// ModeFail returns conflict
	if err := v2.ImportWithOptions(backupPath, backupPass, ImportOptions{Mode: ImportModeFail}); err != ErrImportConflict {
		t.Errorf("ImportWithOptions ModeFail = %v, expected ErrImportConflict", err)
	}
	// Verify unchanged
	if val, _ := v2.Resolve("existing/key"); val != "current_value" {
		t.Errorf("existing/key after ModeFail = %q, expected current_value", val)
	}

	// ModeSkip skips existing, imports new
	if err := v2.ImportWithOptions(backupPath, backupPass, ImportOptions{Mode: ImportModeSkip}); err != nil {
		t.Fatalf("ImportWithOptions ModeSkip: %v", err)
	}
	if val, _ := v2.Resolve("existing/key"); val != "current_value" {
		t.Errorf("existing/key after ModeSkip = %q, expected current_value", val)
	}
	if val, _ := v2.Resolve("new/key"); val != "imported_value" {
		t.Errorf("new/key after ModeSkip = %q, expected imported_value", val)
	}

	// ModeOverwrite updates existing
	if err := v2.ImportWithOptions(backupPath, backupPass, ImportOptions{Mode: ImportModeOverwrite}); err != nil {
		t.Fatalf("ImportWithOptions ModeOverwrite: %v", err)
	}
	if val, _ := v2.Resolve("existing/key"); val != "original_value" {
		t.Errorf("existing/key after ModeOverwrite = %q, expected original_value", val)
	}
}

func TestHTTPImportModes(t *testing.T) {
	dir := t.TempDir()
	v1, _ := New(filepath.Join(dir, "v1.vault"))
	const pass = "correct horse battery staple"
	const backupPass = "backup password string 123"
	_ = v1.Initialize(pass)
	_ = v1.Put("dup/key", "backup_val")
	backupPath := filepath.Join(dir, "backup.env")
	_ = v1.Export(backupPath, backupPass)

	v2, _ := New(filepath.Join(dir, "v2.vault"))
	_ = v2.Initialize(pass)
	_ = v2.Put("dup/key", "target_val")

	ts := httptest.NewServer(HTTP{Vault: v2}.Handler())
	defer ts.Close()

	// Invalid mode via HTTP -> 400
	bodyBad, _ := json.Marshal(map[string]string{
		"backup_path":     backupPath,
		"backup_password": backupPass,
		"mode":            "invalid_mode",
	})
	resp, err := http.Post(ts.URL+"/import", "application/json", bytes.NewReader(bodyBad))
	if err != nil {
		t.Fatalf("POST /import bad mode: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /import bad mode status = %d, expected 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Overwrite mode via HTTP -> 200
	bodyOverwrite, _ := json.Marshal(map[string]string{
		"backup_path":     backupPath,
		"backup_password": backupPass,
		"mode":            "overwrite",
	})
	resp, err = http.Post(ts.URL+"/import", "application/json", bytes.NewReader(bodyOverwrite))
	if err != nil {
		t.Fatalf("POST /import overwrite: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /import overwrite status = %d, expected 200", resp.StatusCode)
	}
	resp.Body.Close()

	if val, _ := v2.Resolve("dup/key"); val != "backup_val" {
		t.Errorf("HTTP import overwrite dup/key = %q, expected backup_val", val)
	}
}
