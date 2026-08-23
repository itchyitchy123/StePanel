package main

import (
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

var restoreStagePattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[a-z0-9_-]{1,32}$`)

func CleanupImportStages(root string, maxAge time.Duration) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() || !restoreStagePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func availableBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
