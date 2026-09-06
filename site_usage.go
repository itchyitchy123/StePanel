package main

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type SiteUsage struct {
	Site        string `json:"site"`
	Bytes       int64  `json:"bytes"`
	Files       int    `json:"files"`
	Directories int    `json:"directories"`
	Complete    bool   `json:"complete"`
}

// siteUsage is deliberately bounded. It reports actual regular-file usage but
// never follows symlinks and refuses to turn a dashboard request into an
// unbounded recursive walk of a tenant-owned tree.
func (a *App) siteUsage(w http.ResponseWriter, r *http.Request) {
	site := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/usage/"), "/")
	if safeUser(site) == "" || strings.Contains(site, "/") || !a.canAccessSite(r, site) {
		http.Error(w, "invalid or inaccessible site", 403)
		return
	}
	root := filepath.Join(a.Config.WebRoot, "sites", site)
	if err := ensureInside(a.Config.WebRoot, root); err != nil {
		http.Error(w, "invalid site root", 422)
		return
	}
	usage, err := measureSiteUsage(root, a.Config.MaxEntries)
	if err != nil && !errors.Is(err, errUsageLimit) {
		http.Error(w, "could not measure site usage", 500)
		return
	}
	writeJSON(w, 200, usage)
}

var errUsageLimit = errors.New("usage entry limit reached")

func measureSiteUsage(root string, limit int) (SiteUsage, error) {
	u := SiteUsage{Site: filepath.Base(root), Complete: true}
	if limit <= 0 {
		limit = 1000000
	}
	entries := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > limit {
			u.Complete = false
			return errUsageLimit
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			u.Directories++
			return nil
		}
		if info.Mode().IsRegular() {
			u.Files++
			u.Bytes += info.Size()
		}
		return nil
	})
	return u, err
}
