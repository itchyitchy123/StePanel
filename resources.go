package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResourceProfile is enforced for managed systemd applications/workers through
// a per-site slice. PHP workers are separately applied to the isolated FPM
// pool. Filesystem, network and database limits require their own providers.
type ResourceProfile struct {
	Site       string    `json:"site"`
	CPUPercent int       `json:"cpu_percent"`
	MemoryMB   int       `json:"memory_mb"`
	TasksMax   int       `json:"tasks_max"`
	PHPWorkers int       `json:"php_workers"`
	AppliedAt  time.Time `json:"applied_at,omitempty"`
	State      string    `json:"state"`
}
type ResourceStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]ResourceProfile
}

func OpenResourceStore(path string) (*ResourceStore, error) {
	s := &ResourceStore{path: path, values: map[string]ResourceProfile{}}
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
func (s *ResourceStore) persistLocked() error {
	d, e := json.MarshalIndent(s.values, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(s.path, append(d, '\n'), 0600)
}
func validResourceProfile(p ResourceProfile) bool {
	return safeUser(p.Site) != "" && p.CPUPercent >= 25 && p.CPUPercent <= 6400 && p.MemoryMB >= 64 && p.MemoryMB <= 1048576 && p.TasksMax >= 16 && p.TasksMax <= 100000 && p.PHPWorkers >= 1 && p.PHPWorkers <= 512
}
func (a *App) siteResources(w http.ResponseWriter, r *http.Request) {
	site := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/resources/"), "/")
	if safeUser(site) == "" || strings.Contains(site, "/") || !a.canAccessSite(r, site) {
		http.Error(w, "invalid or inaccessible site", 403)
		return
	}
	if a.Resources == nil {
		http.Error(w, "resource state unavailable", 503)
		return
	}
	if r.Method == http.MethodGet {
		a.Resources.mu.RLock()
		p, ok := a.Resources.values[site]
		a.Resources.mu.RUnlock()
		writeJSON(w, 200, map[string]any{"configured": ok, "profile": p, "enforcement": "systemd slice for managed apps/workers; isolated PHP-FPM max_children"})
		return
	}
	if r.Method != http.MethodPut || !a.Auth.CSRF(r) || !a.Auth.IsAdministrator(r) {
		http.Error(w, "administrator CSRF request required", 403)
		return
	}
	var p ResourceProfile
	if e := decodeJSON(w, r, 4096, &p); e != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	p.Site = site
	p.State = "pending"
	if !validResourceProfile(p) {
		http.Error(w, "invalid resource profile", 422)
		return
	}
	a.Resources.mu.Lock()
	a.Resources.values[site] = p
	e := a.Resources.persistLocked()
	a.Resources.mu.Unlock()
	if e != nil {
		http.Error(w, "could not persist desired resource profile", 503)
		return
	}
	e = runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "resource-apply", site, strconv.Itoa(p.CPUPercent), strconv.Itoa(p.MemoryMB), strconv.Itoa(p.TasksMax))
	if e == nil {
		e = runHelperCommand(r.Context(), a.Config, a.Config.SiteCtl, "resources", site, strconv.Itoa(p.PHPWorkers))
	}
	if e != nil {
		http.Error(w, "resource profile is pending reconciliation", 502)
		return
	}
	p.State = "applied"
	p.AppliedAt = time.Now().UTC()
	a.Resources.mu.Lock()
	a.Resources.values[site] = p
	e = a.Resources.persistLocked()
	a.Resources.mu.Unlock()
	if e != nil {
		http.Error(w, "resource profile applied but state update failed", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.resources.applied", site, "cgroup/FPM profile")
	writeJSON(w, 202, p)
}
