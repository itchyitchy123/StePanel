package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type DoctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (a *App) doctor(w http.ResponseWriter, _ *http.Request) {
	cfg := a.Config
	services := ServiceStatus()
	checks := []DoctorCheck{}
	web := cfg.WebServer
	if web == "" {
		web = "caddy"
	}
	unit := map[string]string{"apache": "apache2", "openlitespeed": "lsws", "caddy": "caddy"}[web]
	state := services[unit]
	if unit == "apache2" && state == "" {
		state = services["httpd"]
	}
	checks = append(checks, doctorService(web, state))
	dbService := cfg.DBEngine
	if dbService == "" || dbService == "mysql" {
		dbService = "mysql"
	}
	dbState := services[dbService]
	if dbService == "mysql" && dbState == "" {
		dbState = services["mariadb"]
	}
	if dbService == "postgresql" && dbState == "" {
		dbState = services["postgres"]
	}
	checks = append(checks, doctorService(dbService, dbState))
	checks = append(checks, doctorService("php-fpm", services["php-fpm"]))
	checks = append(checks, a.productionReadinessChecks()...)
	if cfg.OffsiteTarget != "" {
		if err := validateOffsiteTarget(cfg.OffsiteTarget); err != nil {
			checks = append(checks, DoctorCheck{"offsite-backup", "fail", "high", err.Error()})
		} else if _, err := exec.LookPath("rclone"); err != nil {
			checks = append(checks, DoctorCheck{"offsite-backup", "fail", "high", "rclone is not installed"})
		} else {
			checks = append(checks, DoctorCheck{"offsite-backup", "pass", "low", "rclone is available and an offsite target is configured"})
		}
	}
	for _, path := range []string{cfg.AppCtl, cfg.ProxyCtl, cfg.SiteCtl, cfg.VHostCtl} {
		if path == "" {
			continue
		}
		name := filepath.Base(path)
		info, err := os.Lstat(path)
		if err != nil {
			checks = append(checks, DoctorCheck{name, "fail", "high", fmt.Sprintf("helper unavailable: %v", err)})
			continue
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			checks = append(checks, DoctorCheck{name, "fail", "critical", "helper is not a regular file"})
			continue
		}
		if info.Mode()&022 != 0 {
			checks = append(checks, DoctorCheck{name, "warn", "high", "helper is writable by group or other users"})
			continue
		}
		if info.Mode()&0111 == 0 {
			checks = append(checks, DoctorCheck{name, "fail", "high", "helper is not executable"})
			continue
		}
		checks = append(checks, DoctorCheck{name, "pass", "low", "executable is present and not group-writable"})
	}
	free, err := availableBytes(cfg.WebRoot)
	if err != nil {
		checks = append(checks, DoctorCheck{"web-root-disk", "fail", "high", err.Error()})
	} else if free < cfg.MinFreeBytes {
		checks = append(checks, DoctorCheck{"web-root-disk", "warn", "high", fmt.Sprintf("%d bytes free; minimum is %d", free, cfg.MinFreeBytes)})
	} else {
		checks = append(checks, DoctorCheck{"web-root-disk", "pass", "low", fmt.Sprintf("%d bytes free", free)})
	}
	failed := 0
	for _, c := range checks {
		if c.Status == "fail" {
			failed++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": checks, "failed": failed, "healthy": failed == 0, "time": time.Now().UTC()})
}

// productionReadinessChecks makes the controls that are easy to miss during a
// first installation visible to automation and to the operator dashboard. It
// intentionally does not claim that an offsite target is immutable or that a
// reverse proxy is correctly configured; those require provider and network
// evidence outside this process.
func (a *App) productionReadinessChecks() []DoctorCheck {
	if !a.Config.Production {
		return []DoctorCheck{{"production-profile", "warn", "medium", "development profile is active; production launch checks are not enforced"}}
	}
	checks := []DoctorCheck{}
	if a.Auth.TOTPEnabled {
		checks = append(checks, DoctorCheck{"administrator-mfa", "pass", "low", "TOTP is required for administrator login"})
	} else {
		checks = append(checks, DoctorCheck{"administrator-mfa", "fail", "high", "production launch requires STEPANEL_ADMIN_TOTP_SECRET"})
	}
	if a.Config.RequireOffsiteBackup && a.Config.OffsiteTarget != "" {
		checks = append(checks, DoctorCheck{"offsite-backup-policy", "pass", "low", "production startup requires an offsite backup target"})
	} else {
		checks = append(checks, DoctorCheck{"offsite-backup-policy", "fail", "high", "set STEPANEL_OFFSITE_TARGET and STEPANEL_REQUIRE_OFFSITE_BACKUP=1 before production launch"})
	}
	switch {
	case a.Config.TLSCertFile != "" || a.Config.TLSKeyFile != "":
		if a.Config.TLSCertFile == "" || a.Config.TLSKeyFile == "" {
			checks = append(checks, DoctorCheck{"transport-security", "fail", "high", "both TLS certificate and key must be configured"})
		} else if _, err := tls.LoadX509KeyPair(a.Config.TLSCertFile, a.Config.TLSKeyFile); err != nil {
			checks = append(checks, DoctorCheck{"transport-security", "fail", "high", fmt.Sprintf("TLS certificate/key cannot be loaded: %v", err)})
		} else {
			checks = append(checks, DoctorCheck{"transport-security", "pass", "low", "the control plane serves a readable, matching application certificate and key"})
		}
	case a.Config.TLSAlreadyTerminated:
		checks = append(checks, DoctorCheck{"transport-security", "warn", "medium", "TLS termination is asserted by configuration; verify the proxy redirects HTTP and protects cookies"})
	default:
		checks = append(checks, DoctorCheck{"transport-security", "warn", "medium", "control plane relies on a loopback listener; verify its external reverse proxy enforces HTTPS"})
	}
	if err := AuditPersistenceError(); err != nil {
		checks = append(checks, DoctorCheck{"audit-persistence", "fail", "critical", err.Error()})
	} else {
		checks = append(checks, DoctorCheck{"audit-persistence", "pass", "low", "audit chain is writable"})
	}
	return checks
}

func doctorService(name, state string) DoctorCheck {
	if state == "active" {
		return DoctorCheck{name, "pass", "low", "service is active"}
	}
	if state == "" || state == "missing" {
		return DoctorCheck{name, "fail", "critical", "service is not installed or was not detected"}
	}
	return DoctorCheck{name, "warn", "high", "service state is " + state}
}
