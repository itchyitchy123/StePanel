package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type BackupSchedule struct {
	Site             string     `json:"site"`
	IntervalMinutes  int        `json:"interval_minutes"`
	IncludeDatabases bool       `json:"include_databases"`
	Enabled          bool       `json:"enabled"`
	NextRun          time.Time  `json:"next_run"`
	LastRun          *time.Time `json:"last_run,omitempty"`
}

type backupSchedules struct {
	mu    sync.Mutex
	path  string
	items map[string]BackupSchedule
}

func openBackupSchedules(path string) (*backupSchedules, error) {
	s := &backupSchedules{path: path, items: map[string]BackupSchedule{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 1<<20 {
		return nil, errors.New("backup schedule state exceeds 1 MiB")
	}
	if err := json.Unmarshal(b, &s.items); err != nil {
		return nil, fmt.Errorf("decode backup schedules: %w", err)
	}
	for site, item := range s.items {
		if safeUser(site) == "" || item.Site != site || item.IntervalMinutes < 5 || item.IntervalMinutes > 10080 || item.NextRun.IsZero() {
			return nil, fmt.Errorf("invalid backup schedule for site %q", site)
		}
	}
	return s, nil
}
func (s *backupSchedules) persistLocked() error {
	b, err := json.Marshal(s.items)
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(b, '\n'), 0600)
}
func (s *backupSchedules) list() []BackupSchedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]BackupSchedule, 0, len(s.items))
	for _, v := range s.items {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Site < out[j].Site })
	return out
}

func (a *App) backupSchedules(w http.ResponseWriter, r *http.Request) {
	if a.Schedules == nil {
		http.Error(w, "backup scheduling is unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"schedules": a.Schedules.list()})
	case http.MethodPut:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		var in BackupSchedule
		if err := decodeJSON(w, r, 4096, &in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		in.Site = safeUser(in.Site)
		if in.Site == "" || in.IntervalMinutes < 5 || in.IntervalMinutes > 10080 {
			http.Error(w, "site is invalid or interval must be 5-10080 minutes", 422)
			return
		}
		if in.IncludeDatabases && a.Config.DBCtl == "" {
			http.Error(w, "managed database backup requires the local database helper", 422)
			return
		}
		if info, err := os.Stat(filepath.Join(a.Config.WebRoot, "sites", in.Site, "public")); err != nil || !info.IsDir() {
			http.Error(w, "site document root does not exist", 422)
			return
		}
		in.Enabled = true
		in.NextRun = time.Now().UTC().Add(time.Duration(in.IntervalMinutes) * time.Minute)
		a.Schedules.mu.Lock()
		a.Schedules.items[in.Site] = in
		err := a.Schedules.persistLocked()
		a.Schedules.mu.Unlock()
		if err != nil {
			http.Error(w, "could not persist backup schedule", 500)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.Username, "backup.schedule.updated", in.Site, fmt.Sprintf("every %d minutes", in.IntervalMinutes))
		writeJSON(w, http.StatusOK, in)
	case http.MethodDelete:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		site := safeUser(r.URL.Query().Get("site"))
		if site == "" {
			http.Error(w, "invalid site", 422)
			return
		}
		a.Schedules.mu.Lock()
		delete(a.Schedules.items, site)
		err := a.Schedules.persistLocked()
		a.Schedules.mu.Unlock()
		if err != nil {
			http.Error(w, "could not persist backup schedule", 500)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.Username, "backup.schedule.deleted", site, "schedule removed")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (a *App) runDueBackups() {
	if a.Schedules == nil {
		return
	}
	now := time.Now().UTC()
	a.Schedules.mu.Lock()
	defer a.Schedules.mu.Unlock()
	for site, s := range a.Schedules.items {
		if !s.Enabled || s.NextRun.After(now) {
			continue
		}
		id, err := newJobID("scheduled-backup")
		if err != nil {
			continue
		}
		s.LastRun = &now
		s.NextRun = now.Add(time.Duration(s.IntervalMinutes) * time.Minute)
		err = a.Jobs.SubmitBackup(id, site, func() (BackupResult, error) {
			result, err := CreateSiteBackup(a.Config, site, s.IncludeDatabases)
			if err != nil {
				_ = AuditAs(a.Config.AuditLog, "scheduler", "site.backup.failed", site, err.Error())
			} else if auditErr := AuditAs(a.Config.AuditLog, "scheduler", "site.backup.completed", site, result.ArchiveSHA256); auditErr != nil {
				err = auditErr
			}
			return result, err
		})
		if err != nil {
			continue
		}
		a.Schedules.items[site] = s
		if err := a.Schedules.persistLocked(); err != nil {
			_ = AuditAs(a.Config.AuditLog, "scheduler", "backup.schedule.persistence_failed", site, err.Error())
		}
	}
}
