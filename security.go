package main

import (
	"fmt"
	"os"
)

type SecurityCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (a *App) SecurityChecks() []SecurityCheck {
	checks := make([]SecurityCheck, 0, 6)
	if a.Auth.Enabled {
		checks = append(checks, SecurityCheck{Name: "Administrator authentication", Status: "pass", Severity: "low", Detail: "Password authentication and signed sessions are enabled."})
	} else {
		severity := "warning"
		status := "warning"
		detail := "Authentication is disabled for local development."
		if a.Config.Production {
			severity, status = "high", "fail"
			detail = "Production mode requires administrator authentication."
		}
		checks = append(checks, SecurityCheck{Name: "Administrator authentication", Status: status, Severity: severity, Detail: detail})
	}
	checks = append(checks, privateDirectoryCheck("Import staging permissions", a.Config.ImportRoot))
	checks = append(checks, directoryCheck("Web root availability", a.Config.WebRoot))

	services := ServiceStatus()
	if services["modsecurity"] == "enabled" {
		checks = append(checks, SecurityCheck{Name: "ModSecurity", Status: "pass", Severity: "low", Detail: "Apache reports the security module as enabled."})
	} else {
		checks = append(checks, SecurityCheck{Name: "ModSecurity", Status: "warning", Severity: "medium", Detail: "ModSecurity was not detected as enabled."})
	}
	if services["fail2ban"] == "installed" || services["fail2ban"] == "active" {
		checks = append(checks, SecurityCheck{Name: "Fail2Ban", Status: "pass", Severity: "low", Detail: "The Fail2Ban executable is available."})
	} else {
		checks = append(checks, SecurityCheck{Name: "Fail2Ban", Status: "warning", Severity: "medium", Detail: "Fail2Ban is not available on PATH."})
	}
	return checks
}

func privateDirectoryCheck(name, path string) SecurityCheck {
	info, err := os.Stat(path)
	if err != nil {
		return SecurityCheck{Name: name, Status: "warning", Severity: "medium", Detail: fmt.Sprintf("%s is unavailable: %v", path, err)}
	}
	if !info.IsDir() {
		return SecurityCheck{Name: name, Status: "fail", Severity: "high", Detail: path + " is not a directory."}
	}
	if info.Mode().Perm()&0077 != 0 {
		return SecurityCheck{Name: name, Status: "warning", Severity: "medium", Detail: path + " should be readable only by its owner."}
	}
	return SecurityCheck{Name: name, Status: "pass", Severity: "low", Detail: path + " is owner-only."}
}

func directoryCheck(name, path string) SecurityCheck {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return SecurityCheck{Name: name, Status: "warning", Severity: "medium", Detail: "The configured web root is not currently available."}
	}
	return SecurityCheck{Name: name, Status: "pass", Severity: "low", Detail: path + " is available."}
}
