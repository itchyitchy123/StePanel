package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"time"
)

type ReadinessCheck struct {
	Ready  bool   `json:"ready"`
	Detail string `json:"detail,omitempty"`
}

func (a *App) livez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) readyz(w http.ResponseWriter, _ *http.Request) {
	checks := readinessChecks(a.Config, a.Jobs)
	ready := true
	for _, check := range checks {
		if !check.Ready {
			ready = false
			break
		}
	}
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{"ready": ready, "checks": checks, "time": time.Now().UTC()})
}

func readinessChecks(cfg Config, jobs *Jobs) map[string]ReadinessCheck {
	checks := map[string]ReadinessCheck{}
	if err := AuditPersistenceError(); err != nil {
		checks["audit_state"] = ReadinessCheck{Ready: false, Detail: err.Error()}
	} else {
		checks["audit_state"] = ReadinessCheck{Ready: true}
	}
	if jobs == nil {
		checks["job_state"] = ReadinessCheck{Ready: false, Detail: "job store is not initialized"}
	} else if err := jobs.PersistenceError(); err != nil {
		checks["job_state"] = ReadinessCheck{Ready: false, Detail: err.Error()}
	} else {
		checks["job_state"] = ReadinessCheck{Ready: true}
	}
	roots := map[string]string{
		"backup_capacity":   cfg.BackupRoot,
		"import_capacity":   cfg.ImportRoot,
		"job_capacity":      filepath.Dir(cfg.JobState),
		"recovery_capacity": filepath.Dir(cfg.RecoveryRoot),
	}
	names := make([]string, 0, len(roots))
	for name := range roots {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := roots[name]
		free, err := availableBytes(path)
		switch {
		case err != nil:
			checks[name] = ReadinessCheck{Ready: false, Detail: err.Error()}
		case free < cfg.MinFreeBytes:
			checks[name] = ReadinessCheck{Ready: false, Detail: fmt.Sprintf("%d bytes free; minimum is %d", free, cfg.MinFreeBytes)}
		default:
			checks[name] = ReadinessCheck{Ready: true, Detail: fmt.Sprintf("%d bytes free", free)}
		}
	}
	return checks
}

func restoreCapacity(cfg Config) error {
	paths := []string{cfg.ImportRoot, filepath.Join(cfg.WebRoot, "sites")}
	checked := map[string]bool{}
	for _, path := range paths {
		if checked[path] {
			continue
		}
		checked[path] = true
		free, err := availableBytes(path)
		if err != nil {
			return fmt.Errorf("inspect restore capacity at %s: %w", path, err)
		}
		if free < cfg.MinFreeBytes {
			return fmt.Errorf("insufficient free space at %s: %d bytes available, %d required", path, free, cfg.MinFreeBytes)
		}
	}
	return nil
}
