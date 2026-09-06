package main

import (
	"context"
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
	Site         string    `json:"site"`
	CPUPercent   int       `json:"cpu_percent"`
	CPUWeight    int       `json:"cpu_weight,omitempty"`
	MemoryHighMB int       `json:"memory_high_mb,omitempty"`
	MemoryMB     int       `json:"memory_mb"`
	IOWeight     int       `json:"io_weight,omitempty"`
	TasksMax     int       `json:"tasks_max"`
	PHPWorkers   int       `json:"php_workers"`
	AppliedAt    time.Time `json:"applied_at,omitempty"`
	State        string    `json:"state"`
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
	for site, profile := range s.values {
		profile = normalizeResourceProfile(profile)
		s.values[site] = profile
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
func normalizeResourceProfile(p ResourceProfile) ResourceProfile {
	if p.CPUWeight == 0 {
		p.CPUWeight = 100
	}
	if p.MemoryHighMB == 0 {
		p.MemoryHighMB = p.MemoryMB * 90 / 100
		if p.MemoryHighMB < 64 {
			p.MemoryHighMB = 64
		}
	}
	if p.IOWeight == 0 {
		p.IOWeight = 100
	}
	return p
}
func validResourceProfile(p ResourceProfile) bool {
	return safeUser(p.Site) != "" && p.CPUPercent >= 25 && p.CPUPercent <= 6400 && p.CPUWeight >= 1 && p.CPUWeight <= 10000 && p.MemoryMB >= 64 && p.MemoryHighMB >= 64 && p.MemoryHighMB <= p.MemoryMB && p.MemoryMB <= 1048576 && p.IOWeight >= 1 && p.IOWeight <= 10000 && p.TasksMax >= 16 && p.TasksMax <= 100000 && p.PHPWorkers >= 1 && p.PHPWorkers <= 512
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
		observed := map[string]string{"state": "unknown"}
		if ok {
			observed = a.resourceObserved(r.Context(), site)
		}
		writeJSON(w, 200, map[string]any{"configured": ok, "profile": p, "observed": observed, "enforcement": "systemd slice for managed apps/workers; isolated PHP-FPM max_children"})
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
	p = normalizeResourceProfile(p)
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
	e = runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "resource-apply", site, strconv.Itoa(p.CPUPercent), strconv.Itoa(p.CPUWeight), strconv.Itoa(p.MemoryHighMB), strconv.Itoa(p.MemoryMB), strconv.Itoa(p.IOWeight), strconv.Itoa(p.TasksMax))
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

func (a *App) resourceObserved(ctx context.Context, site string) map[string]string {
	output, err := runBoundedCommand(ctx, helperCommandContext(ctx, a.Config, a.Config.AppCtl, "resource-status", site))
	if err != nil {
		return map[string]string{"state": "unavailable"}
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	if values["ActiveState"] == "" {
		values["state"] = "unknown"
	} else {
		values["state"] = values["ActiveState"]
	}
	return values
}

func (a *App) reconcileResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) || !a.Auth.IsAdministrator(r) {
		http.Error(w, "administrator CSRF request required", 403)
		return
	}
	if a.Resources == nil {
		http.Error(w, "resource state unavailable", 503)
		return
	}
	a.Resources.mu.RLock()
	pending := make([]ResourceProfile, 0, len(a.Resources.values))
	for _, p := range a.Resources.values {
		if p.State != "applied" || a.resourceObserved(r.Context(), p.Site)["state"] != "active" {
			pending = append(pending, p)
		}
	}
	a.Resources.mu.RUnlock()
	reconciled, failed := []string{}, map[string]string{}
	for _, p := range pending {
		err := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "resource-apply", p.Site, strconv.Itoa(p.CPUPercent), strconv.Itoa(p.CPUWeight), strconv.Itoa(p.MemoryHighMB), strconv.Itoa(p.MemoryMB), strconv.Itoa(p.IOWeight), strconv.Itoa(p.TasksMax))
		if err == nil {
			err = runHelperCommand(r.Context(), a.Config, a.Config.SiteCtl, "resources", p.Site, strconv.Itoa(p.PHPWorkers))
		}
		if err != nil {
			failed[p.Site] = "apply failed"
			continue
		}
		p.State = "applied"
		p.AppliedAt = time.Now().UTC()
		a.Resources.mu.Lock()
		a.Resources.values[p.Site] = p
		err = a.Resources.persistLocked()
		a.Resources.mu.Unlock()
		if err != nil {
			failed[p.Site] = "state persistence failed"
			continue
		}
		reconciled = append(reconciled, p.Site)
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "resources.reconciled", "resources", strings.Join(reconciled, ","))
	writeJSON(w, 200, map[string]any{"reconciled": reconciled, "failed": failed})
}
