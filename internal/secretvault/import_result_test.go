package secretvault

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestImportResultAndHTTPResponse(t *testing.T) {
	dir := t.TempDir()
	v1Path := filepath.Join(dir, "v1.vault")
	v2Path := filepath.Join(dir, "v2.vault")
	backupPath := filepath.Join(dir, "backup.env")
	const pass = "correct horse battery staple"
	const backupPass = "backup password string 123"

	v1, err := New(v1Path)
	if err != nil {
		t.Fatalf("New v1: %v", err)
	}
	_ = v1.Initialize(pass)
	_ = v1.Put("key1", "val1")
	_ = v1.Put("key2", "val2")
	if err := v1.Export(backupPath, backupPass); err != nil {
		t.Fatalf("Export v1: %v", err)
	}

	v2, err := New(v2Path)
	if err != nil {
		t.Fatalf("New v2: %v", err)
	}
	_ = v2.Initialize(pass)
	_ = v2.Put("key1", "existing_val1")

	// HTTP POST /import with mode=skip
	srv := httptest.NewServer(HTTP{Vault: v2}.Handler())
	defer srv.Close()

	bodyBytes, _ := json.Marshal(map[string]string{
		"backup_path":     backupPath,
		"backup_password": backupPass,
		"mode":            "skip",
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/import", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /import: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, expected 200", resp.StatusCode)
	}

	var res ImportResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Decode HTTP import result: %v", err)
	}
	if res.Total != 2 || res.Imported != 1 || res.Skipped != 1 {
		t.Errorf("HTTP import result = %+v, expected Total=2, Imported=1, Skipped=1", res)
	}
}
