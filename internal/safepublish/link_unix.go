//go:build unix

package safepublish

import (
	"os"

	"golang.org/x/sys/unix"
)

func linkAtRoot(root *os.Root, oldName, newName string) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return unix.Linkat(int(directory.Fd()), oldName, int(directory.Fd()), newName, 0)
}
