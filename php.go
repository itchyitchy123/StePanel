package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type PHPProfile struct {
	Site              string `json:"site"`
	Version           string `json:"version"`
	MemoryLimit       string `json:"memory_limit"`
	MaxExecutionTime  int    `json:"max_execution_time"`
	UploadMaxFilesize string `json:"upload_max_filesize"`
	PostMaxSize       string `json:"post_max_size"`
	MaxInputVars      int    `json:"max_input_vars"`
	OPcache           bool   `json:"opcache"`
	DisplayErrors     bool   `json:"display_errors"`
	ErrorReporting    string `json:"error_reporting"`
}
type PHPProfileStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]PHPProfile
}

var phpSizePattern = regexp.MustCompile(`^[1-9][0-9]{0,4}M$`)

func OpenPHPProfileStore(path string) (*PHPProfileStore, error) {
	s := &PHPProfileStore{path: path, values: map[string]PHPProfile{}}
	d, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(d, &s.values); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *PHPProfileStore) save(site string, p PHPProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[site] = p
	d, e := json.MarshalIndent(s.values, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(s.path, append(d, '\n'), 0600)
}
func (s *PHPProfileStore) get(site string) (PHPProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.values[site]
	return p, ok
}
func installedPHPVersions() []string {
	entries, _ := filepath.Glob("/etc/php/*/fpm/pool.d")
	out := []string{}
	for _, entry := range entries {
		v := filepath.Base(filepath.Dir(filepath.Dir(entry)))
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func (a *App) phpRuntime(w http.ResponseWriter, r *http.Request) {
	site := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/php/"), "/")
	if site == "" || strings.Contains(site, "/") || safeUser(site) == "" {
		http.Error(w, "invalid site", 422)
		return
	}
	if !a.canAccessSite(r, site) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	if r.Method == http.MethodGet {
		p, ok := a.PHP.get(site)
		writeJSON(w, 200, map[string]any{"site": site, "profile": p, "configured": ok, "versions": installedPHPVersions(), "extensions": []string{"curl", "gd", "intl", "mbstring", "mysqli", "opcache", "zip"}})
		return
	}
	if r.Method != http.MethodPut || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var p PHPProfile
	if e := decodeJSON(w, r, 4096, &p); e != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	p.Site = site
	p.Version = strings.TrimPrefix(p.Version, "v")
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+$`).MatchString(p.Version) || !phpSizePattern.MatchString(p.MemoryLimit) || !phpSizePattern.MatchString(p.UploadMaxFilesize) || !phpSizePattern.MatchString(p.PostMaxSize) || p.MaxExecutionTime < 1 || p.MaxExecutionTime > 3600 || p.MaxInputVars < 1 || p.MaxInputVars > 1000000 || !regexp.MustCompile(`^[A-Z0-9_~& |]{1,80}$`).MatchString(p.ErrorReporting) {
		http.Error(w, "invalid PHP profile", 422)
		return
	}
	if e := runHelperCommand(r.Context(), a.Config, a.Config.SiteCtl, "runtime", site, p.Version, p.MemoryLimit, itoa(p.MaxExecutionTime), p.UploadMaxFilesize, p.PostMaxSize, itoa(p.MaxInputVars), boolString(p.OPcache), boolString(p.DisplayErrors), p.ErrorReporting); e != nil {
		http.Error(w, "PHP runtime helper rejected the profile", 502)
		return
	}
	if e := a.PHP.save(site, p); e != nil {
		http.Error(w, "PHP profile applied but state could not be saved", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.php.updated", site, p.Version)
	writeJSON(w, 202, p)
}
func itoa(v int) string { return fmt.Sprintf("%d", v) }
