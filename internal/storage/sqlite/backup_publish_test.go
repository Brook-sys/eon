package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishBackupNoReplaceRejectsDestinationCreatedAfterPreflight(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, ".sqlite-backup-temp")
	destPath := filepath.Join(dir, "runtime.sqlite")
	if err := os.WriteFile(tempPath, []byte("verified backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destPath, []byte("preserve existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := publishBackupNoReplace(tempPath, destPath); err == nil {
		t.Fatal("publish overwrote a destination created after preflight")
	}
	contents, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "preserve existing" {
		t.Fatalf("destination changed: %q", contents)
	}
	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("failed publication should preserve temp for caller cleanup: %v", err)
	}
}

func TestPublishBackupNoReplacePublishesRestrictedInode(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, ".sqlite-backup-temp")
	destPath := filepath.Join(dir, "runtime.sqlite")
	if err := os.WriteFile(tempPath, []byte("verified backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := publishBackupNoReplace(tempPath, destPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("temporary name remains after publication: %v", err)
	}
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("published mode = %04o, want 0600", got)
	}
}

func TestSyncRegularFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.sqlite")
	if err := os.WriteFile(path, []byte("durable backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syncRegularFile(path); err != nil {
		t.Fatalf("sync regular file: %v", err)
	}
	if err := syncDirectory(dir); err != nil {
		t.Fatalf("sync directory: %v", err)
	}
}
