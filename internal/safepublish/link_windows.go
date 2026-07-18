//go:build windows

package safepublish

import (
	"os"
	"path/filepath"
)

func linkAtRoot(root *os.Root, oldName, newName string) error {
	return os.Link(filepath.Join(root.Name(), oldName), filepath.Join(root.Name(), newName))
}
