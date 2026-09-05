package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

func pruneSiteBackups(root, site string, keep int) error {
	if safeUser(site) == "" || keep < 1 {
		return errors.New("invalid backup retention policy")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type candidate struct{ name, path string }
	items := []candidate{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		path := filepath.Join(root, entry.Name())
		manifest, readErr := readBackupManifest(path)
		if readErr == nil && manifest.Site == site {
			items = append(items, candidate{entry.Name(), path})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].name > items[j].name })
	for _, item := range items[keep:] {
		if err := os.RemoveAll(item.path); err != nil {
			return err
		}
	}
	return syncDirectory(root)
}
