// Package state contains small durability primitives shared by control-plane
// state stores. It intentionally does not know about StePanel domains.
package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes data to path with the requested permissions, fsyncs the
// file, atomically replaces the destination, and fsyncs its parent directory.
// The temporary file is removed on every error path.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	root := filepath.Dir(path)
	if err := os.MkdirAll(root, 0750); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	tmp, err := os.CreateTemp(root, ".stepanel-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set state file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write state file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close state file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	dir, err := os.Open(root)
	if err != nil {
		return fmt.Errorf("open state directory for sync: %w", err)
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return fmt.Errorf("sync state directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close state directory: %w", closeErr)
	}
	return nil
}
