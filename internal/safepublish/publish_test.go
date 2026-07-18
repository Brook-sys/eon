package safepublish

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNoReplacePublishesRestrictedFile(t *testing.T) {
	dir := t.TempDir()
	tempPath := filepath.Join(dir, ".temporary")
	destinationPath := filepath.Join(dir, "artifact")
	if err := os.WriteFile(tempPath, []byte("verified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NoReplace(tempPath, destinationPath, "test artifact"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("temporary name remains: %v", err)
	}
	info, err := os.Stat(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestNoReplaceFailsClosedWhenParentPathIsReplaced(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "inventory")
	moved := filepath.Join(base, "inventory-moved")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tempPath := filepath.Join(dir, ".temporary")
	destinationPath := filepath.Join(dir, "artifact")
	if err := os.WriteFile(tempPath, []byte("verified"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := noReplace(tempPath, destinationPath, "test artifact", func() {
		if renameErr := os.Rename(dir, moved); renameErr != nil {
			t.Fatalf("rename parent: %v", renameErr)
		}
		if mkdirErr := os.Mkdir(dir, 0o755); mkdirErr != nil {
			t.Fatalf("replace parent: %v", mkdirErr)
		}
	})
	if err == nil {
		t.Fatal("publication succeeded after parent path replacement")
	}
	if _, statErr := os.Lstat(destinationPath); !os.IsNotExist(statErr) {
		t.Fatalf("artifact appeared in replacement directory: %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(moved, "artifact")); !os.IsNotExist(statErr) {
		t.Fatalf("artifact remained in moved directory after rollback: %v", statErr)
	}
}
