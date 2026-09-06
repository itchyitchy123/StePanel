package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResourceProfile is enforced for managed systemd applications/workers through
// an optional aggregate account slice and a per-site child slice. PHP workers
// are separately applied to the isolated FPM pool. Filesystem, network and
// database limits require their own providers.
type ResourceProfile struct {
	Account              string    `json:"account,omitempty"`
	Site                 string    `json:"site"`
	CPUPercent           int       `json:"cpu_percent"`
	CPUWeight            int       `json:"cpu_weight,omitempty"`
	MemoryHighMB         int       `json:"memory_high_mb,omitempty"`
	MemoryMB             int       `json:"memory_mb"`
	IOWeight             int       `json:"io_weight,omitempty"`
	TasksMax             int       `json:"tasks_max"`
	PHPWorkers           int       `json:"php_workers"`
	DiskMB               int       `json:"disk_mb,omitempty"`
	Inodes               int       `json:"inodes,omitempty"`
	FilesystemQuotaState string    `json:"filesystem_quota_state,omitempty"`
	AppliedAt            time.Time `json:"applied_at,omitempty"`
	State                string    `json:"state"`
}
type ResourceStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]ResourceProfile
}

func resourceProfileForPlan(account, site string, plan HostingPlan) ResourceProfile {
	high := plan.MemoryMB * 90 / 100
	if high < 64 {
		high = 64
	}
	return ResourceProfile{Account: account, Site: site, CPUPercent: plan.CPUPercent, CPUWeight: 100, MemoryHighMB: high, MemoryMB: plan.MemoryMB, IOWeight: 100, TasksMax: plan.TasksMax, PHPWorkers: plan.PHPWorkers, State: "pending", FilesystemQuotaState: "none"}
}

// ensurePlanResources persists desired resource profiles for newly assigned
// sites without overwriting an administrator's existing site-specific policy.
// Host application is deliberately separate so a helper failure leaves a
// durable pending profile for reconciliation.
func (a *App) ensurePlanResources(account HostingAccount) ([]string, error) {
	if a.Resources == nil {
		return nil, nil
	}
	plan, ok := hostingPlans[account.Plan]
	if !ok {
		return nil, errors.New("account plan is not available")
	}
	profiles := make([]ResourceProfile, 0, len(account.Sites))
	a.Resources.mu.Lock()
	for _, site := range account.Sites {
		if _, exists := a.Resources.values[site]; exists {
			continue
		}
		profile := resourceProfileForPlan(account.Username, site, plan)
		a.Resources.values[site] = profile
		profiles = append(profiles, profile)
	}
	err := a.Resources.persistLocked()
	if err != nil {
		for _, profile := range profiles {
			delete(a.Resources.values, profile.Site)
		}
	}
	a.Resources.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("persist plan resource profiles: %w", err)
	}
	pending := make([]string, 0, len(profiles))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, profile := range profiles {
		if err := a.applyResourceProfile(ctx, profile, false); err != nil {
			pending = append(pending, profile.Site)
			continue
		}
		profile.State = "applied"
		a.Resources.mu.Lock()
		a.Resources.values[profile.Site] = profile
		err = a.Resources.persistLocked()
		a.Resources.mu.Unlock()
		if err != nil {
			pending = append(pending, profile.Site)
		}
	}
	return pending, nil
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
		if profile.Site == "" {
			profile.Site = site
		}
		if profile.Site != site || !validResourceProfile(profile) {
			return nil, fmt.Errorf("invalid resource profile for site %q", site)
		}
		if profile.FilesystemQuotaState == "" {
			if profile.hasFilesystemQuota() {
				profile.FilesystemQuotaState = "enforced"
			} else {
				profile.FilesystemQuotaState = "none"
			}
		}
		if profile.FilesystemQuotaState != "none" && profile.FilesystemQuotaState != "enforced" && profile.FilesystemQuotaState != "apply-pending" && profile.FilesystemQuotaState != "clear-pending" {
			return nil, fmt.Errorf("invalid filesystem quota state for site %q", site)
		}
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
	return safeUser(p.Site) != "" && (p.Account == "" || safeUser(p.Account) != "") && p.CPUPercent >= 25 && p.CPUPercent <= 6400 && p.CPUWeight >= 1 && p.CPUWeight <= 10000 && p.MemoryMB >= 64 && p.MemoryHighMB >= 64 && p.MemoryHighMB <= p.MemoryMB && p.MemoryMB <= 1048576 && p.IOWeight >= 1 && p.IOWeight <= 10000 && p.TasksMax >= 16 && p.TasksMax <= 100000 && p.PHPWorkers >= 1 && p.PHPWorkers <= 512 && (p.DiskMB == 0 && p.Inodes == 0 || p.DiskMB >= 64 && p.DiskMB <= 1048576 && p.Inodes >= 1000 && p.Inodes <= 1000000000)
}

func (p ResourceProfile) hasFilesystemQuota() bool { return p.DiskMB > 0 || p.Inodes > 0 }

func (a *App) applyResourceProfile(ctx context.Context, p ResourceProfile, clearFilesystemQuota bool) error {
	if p.Account != "" {
		if err := runHelperCommand(ctx, a.Config, a.Config.AppCtl, "account-resource-apply", p.Account, strconv.Itoa(p.CPUPercent), strconv.Itoa(p.CPUWeight), strconv.Itoa(p.MemoryHighMB), strconv.Itoa(p.MemoryMB), strconv.Itoa(p.IOWeight), strconv.Itoa(p.TasksMax)); err != nil {
			return err
		}
	}
	if err := runHelperCommand(ctx, a.Config, a.Config.AppCtl, "resource-apply", p.Site, strconv.Itoa(p.CPUPercent), strconv.Itoa(p.CPUWeight), strconv.Itoa(p.MemoryHighMB), strconv.Itoa(p.MemoryMB), strconv.Itoa(p.IOWeight), strconv.Itoa(p.TasksMax), p.Account); err != nil {
		return err
	}
	if err := runHelperCommand(ctx, a.Config, a.Config.SiteCtl, "resources", p.Site, strconv.Itoa(p.PHPWorkers)); err != nil {
		return err
	}
	if p.hasFilesystemQuota() {
		if err := runHelperCommand(ctx, a.Config, a.Config.SiteCtl, "quota", p.Site, strconv.Itoa(p.DiskMB), strconv.Itoa(p.Inodes)); err != nil {
			return err
		}
	} else if clearFilesystemQuota {
		if err := runHelperCommand(ctx, a.Config, a.Config.SiteCtl, "quota-clear", p.Site); err != nil {
			return err
		}
	}
	return nil
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
		writeJSON(w, 200, map[string]any{"configured": ok, "profile": p, "observed": observed, "enforcement": "systemd slice for managed apps/workers/scheduled tasks; isolated PHP-FPM max_children"})
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
	a.Resources.mu.RLock()
	previous, hadPrevious := a.Resources.values[site]
	a.Resources.mu.RUnlock()
	p.Account = ""
	if hadPrevious {
		p.Account = previous.Account
	}
	p = normalizeResourceProfile(p)
	p.State = "pending"
	p.FilesystemQuotaState = "none"
	if p.hasFilesystemQuota() {
		p.FilesystemQuotaState = "apply-pending"
	} else if hadPrevious && previous.FilesystemQuotaState != "none" && previous.FilesystemQuotaState != "" {
		p.FilesystemQuotaState = "clear-pending"
	}
	if !validResourceProfile(p) {
		http.Error(w, "invalid resource profile", 422)
		return
	}
	releaseUnlock := a.siteOperations.acquire(site)
	defer releaseUnlock()
	a.Resources.mu.Lock()
	a.Resources.values[site] = p
	e := a.Resources.persistLocked()
	a.Resources.mu.Unlock()
	if e != nil {
		http.Error(w, "could not persist desired resource profile", 503)
		return
	}
	e = a.applyResourceProfile(r.Context(), p, p.FilesystemQuotaState == "clear-pending")
	if e != nil {
		http.Error(w, "resource profile is pending reconciliation", 502)
		return
	}
	p.State = "applied"
	if p.hasFilesystemQuota() {
		p.FilesystemQuotaState = "enforced"
	} else {
		p.FilesystemQuotaState = "none"
	}
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
	profiles := make([]ResourceProfile, 0, len(a.Resources.values))
	for _, p := range a.Resources.values {
		profiles = append(profiles, p)
	}
	a.Resources.mu.RUnlock()

	pending := make([]ResourceProfile, 0, len(profiles))
	for _, p := range profiles {
		if p.State != "applied" || a.resourceObserved(r.Context(), p.Site)["state"] != "active" {
			pending = append(pending, p)
		}
	}
	reconciled, failed := []string{}, map[string]string{}
	for _, p := range pending {
		releaseUnlock := a.siteOperations.acquire(p.Site)
		err := a.applyResourceProfile(r.Context(), p, p.FilesystemQuotaState == "clear-pending")
		if err != nil {
			failed[p.Site] = "apply failed"
			releaseUnlock()
			continue
		}
		p.State = "applied"
		if p.hasFilesystemQuota() {
			p.FilesystemQuotaState = "enforced"
		} else {
			p.FilesystemQuotaState = "none"
		}
		p.AppliedAt = time.Now().UTC()
		a.Resources.mu.Lock()
		a.Resources.values[p.Site] = p
		err = a.Resources.persistLocked()
		a.Resources.mu.Unlock()
		if err != nil {
			failed[p.Site] = "state persistence failed"
			releaseUnlock()
			continue
		}
		reconciled = append(reconciled, p.Site)
		releaseUnlock()
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "resources.reconciled", "resources", strings.Join(reconciled, ","))
	writeJSON(w, 200, map[string]any{"reconciled": reconciled, "failed": failed})
}
