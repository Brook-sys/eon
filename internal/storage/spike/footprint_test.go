package spike

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskFootprintCountsRegularFilesWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one"), []byte("123"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "two"), []byte("45678"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "one"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := DiskFootprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 8 {
		t.Fatalf("footprint = %d, want 8", got)
	}
}

func TestDiskFootprintRejectsMissingRoot(t *testing.T) {
	if _, err := DiskFootprint(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing root was accepted")
	}
}
