// Package safepublish publishes verified temporary files without replacement
// while pinning the containing directory to an open descriptor.
package safepublish

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// NoReplace links tempPath to destinationPath atomically and removes the
// temporary name. Both names must be direct children of the same directory.
// The directory is pinned with os.Root and its path identity is checked before
// and after publication, so a concurrent parent rename or symlink substitution
// fails closed instead of redirecting the artifact.
func NoReplace(tempPath, destinationPath, artifact string) error {
	return noReplace(tempPath, destinationPath, artifact, nil)
}

func noReplace(tempPath, destinationPath, artifact string, beforeLink func()) error {
	tempPath = filepath.Clean(tempPath)
	destinationPath = filepath.Clean(destinationPath)
	dir := filepath.Dir(destinationPath)
	if filepath.Dir(tempPath) != dir {
		return fmt.Errorf("%s temporary and destination paths must share a directory", artifact)
	}
	tempName, destinationName := filepath.Base(tempPath), filepath.Base(destinationPath)
	if tempName == "." || destinationName == "." || tempName == destinationName {
		return fmt.Errorf("invalid %s publication names", artifact)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open %s parent directory: %w", artifact, err)
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("inspect pinned %s parent directory: %w", artifact, err)
	}
	if err := requireDirectoryPathIdentity(dir, rootInfo, artifact); err != nil {
		return err
	}
	tempInfo, err := root.Lstat(tempName)
	if err != nil {
		return fmt.Errorf("inspect pinned %s temporary file: %w", artifact, err)
	}
	pathTempInfo, err := os.Lstat(tempPath)
	if err != nil || !tempInfo.Mode().IsRegular() || pathTempInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(tempInfo, pathTempInfo) {
		return fmt.Errorf("%s temporary path is not the pinned regular file", artifact)
	}
	if beforeLink != nil {
		beforeLink()
	}
	if err := linkAtRoot(root, tempName, destinationName); err != nil {
		if _, statErr := root.Lstat(destinationName); statErr == nil {
			return fmt.Errorf("%s destination already exists: %s", artifact, destinationPath)
		}
		return fmt.Errorf("publish %s without overwrite: %w", artifact, err)
	}
	rollback := func() {
		_ = root.Remove(destinationName)
		_ = syncRoot(root)
	}
	if err := requireDirectoryPathIdentity(dir, rootInfo, artifact); err != nil {
		rollback()
		return err
	}
	if err := syncRoot(root); err != nil {
		rollback()
		return fmt.Errorf("sync published %s directory: %w", artifact, err)
	}
	if err := root.Remove(tempName); err != nil {
		rollback()
		return fmt.Errorf("remove published %s temporary name: %w", artifact, err)
	}
	if err := syncRoot(root); err != nil {
		return fmt.Errorf("sync %s temporary-name removal: %w", artifact, err)
	}
	return nil
}

func requireDirectoryPathIdentity(path string, expected os.FileInfo, artifact string) error {
	actual, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("reinspect %s parent directory: %w", artifact, err)
	}
	if !actual.IsDir() || !os.SameFile(expected, actual) {
		return errors.New(artifact + " parent directory changed during publication")
	}
	return nil
}

func syncRoot(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
