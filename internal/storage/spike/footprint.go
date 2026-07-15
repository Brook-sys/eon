package spike

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// DiskFootprint returns the sum of regular-file logical sizes below root.
// Symlinks are not followed, keeping runs reproducible and confined to the
// backend directory selected by the harness.
func DiskFootprint(root string) (int64, error) {
	if root == "" {
		return 0, fmt.Errorf("footprint root is required")
	}
	var bytes int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			bytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure disk footprint %q: %w", root, err)
	}
	return bytes, nil
}
