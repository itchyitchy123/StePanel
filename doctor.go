package main

import (
	"fmt"
	"net/http"
	"os"
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
		web = "apache"
	}
	unit := map[string]string{"apache": "apache2", "openlitespeed": "lsws", "caddy": "caddy"}[web]
	state := services[unit]
	if unit == "apache2" && state == "" {
		state = services["httpd"]
	}
	checks = append(checks, doctorService(web, state))
	dbState := services["mysql"]
	if dbState == "" {
		dbState = services["mariadb"]
	}
	checks = append(checks, doctorService("database", dbState))
	checks = append(checks, doctorService("php-fpm", services["php-fpm"]))
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

func doctorService(name, state string) DoctorCheck {
	if state == "active" {
		return DoctorCheck{name, "pass", "low", "service is active"}
	}
	if state == "" || state == "missing" {
		return DoctorCheck{name, "fail", "critical", "service is not installed or was not detected"}
	}
	return DoctorCheck{name, "warn", "high", "service state is " + state}
}
