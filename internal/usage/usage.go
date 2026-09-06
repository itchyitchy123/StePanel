// Package usage measures bounded site filesystem usage without following
// symlinks or turning an operator request into an unbounded recursive walk.
package usage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrLimit indicates that the configured entry bound was reached. The result
// remains useful but is marked incomplete.
var ErrLimit = errors.New("usage entry limit reached")

type SiteUsage struct {
	Site        string `json:"site"`
	Bytes       int64  `json:"bytes"`
	Files       int    `json:"files"`
	Directories int    `json:"directories"`
	Complete    bool   `json:"complete"`
}

// Measure returns regular-file usage below root. Symlinks are counted neither
// as files nor as directories, and traversal is bounded by limit.
func Measure(root string, limit int) (SiteUsage, error) {
	result := SiteUsage{Site: filepath.Base(root), Complete: true}
	if limit <= 0 {
		limit = 1000000
	}
	entries := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > limit {
			result.Complete = false
			return ErrLimit
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			result.Directories++
			return nil
		}
		if info.Mode().IsRegular() {
			result.Files++
			result.Bytes += info.Size()
		}
		return nil
	})
	return result, err
}
