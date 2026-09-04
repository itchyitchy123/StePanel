package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ServiceSummary struct {
	Name   string
	Region string
	Status string
	Load   string
	Uptime string
}

var serviceStatusCache struct {
	sync.RWMutex
	at    time.Time
	items map[string]string
}

func ServiceStatus() map[string]string {
	serviceStatusCache.RLock()
	if time.Since(serviceStatusCache.at) < 5*time.Second {
		result := cloneServiceStatus(serviceStatusCache.items)
		serviceStatusCache.RUnlock()
		return result
	}
	serviceStatusCache.RUnlock()
	result := map[string]string{}
	for _, service := range []string{"apache2", "httpd", "mysql", "mariadb", "php-fpm", "fail2ban", "fpm-lens", "exim4", "exim", "dovecot", "spamassassin", "spamd", "vsftpd"} {
		if _, err := exec.LookPath(service); err == nil {
			result[service] = serviceUnitState(service)
		}
	}
	for _, apache := range []string{"apachectl", "httpd"} {
		if _, err := exec.LookPath(apache); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		output, err := exec.CommandContext(ctx, apache, "-M").CombinedOutput()
		cancel()
		if err == nil && (bytes.Contains(output, []byte("security2_module")) || bytes.Contains(output, []byte("security3_module"))) {
			result["modsecurity"] = "enabled"
			break
		}
	}
	serviceStatusCache.Lock()
	serviceStatusCache.items = cloneServiceStatus(result)
	serviceStatusCache.at = time.Now()
	serviceStatusCache.Unlock()
	return result
}

func cloneServiceStatus(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func serviceUnitState(service string) string {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "installed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "systemctl", "is-active", service).CombinedOutput()
	state := strings.TrimSpace(string(output))
	if err == nil && state == "active" {
		return "active"
	}
	if state == "failed" {
		return "failed"
	}
	if state != "" {
		return state
	}
	return "unknown"
}

func ServiceSummaries() []ServiceSummary {
	status := ServiceStatus()
	load := "n/a"
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			load = fields[0]
		}
	}
	uptime := "n/a"
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if seconds, err := strconv.ParseFloat(fields[0], 64); err == nil {
				uptime = formatUptime(seconds)
			}
		}
	}
	services := []string{"apache2", "mysql", "php-fpm", "fail2ban", "modsecurity", "exim", "dovecot", "spamassassin", "vsftpd"}
	result := make([]ServiceSummary, 0, len(services))
	for _, name := range services {
		state := status[name]
		if name == "apache2" && state == "" {
			state = status["httpd"]
		}
		if name == "mysql" && state == "" {
			state = status["mariadb"]
		}
		if name == "exim" && state == "" {
			state = status["exim4"]
		}
		if name == "spamassassin" && state == "" {
			state = status["spamd"]
		}
		if state == "" {
			state = "missing"
		}
		result = append(result, ServiceSummary{Name: name, Region: "this server", Status: state, Load: load, Uptime: uptime})
	}
	return result
}

func (a *App) ftpStatus(w http.ResponseWriter, r *http.Request) {
	status := ServiceStatus()["vsftpd"]
	if status == "" {
		status = "missing"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":           "vsftpd",
		"status":            status,
		"anonymous":         false,
		"local_user_chroot": true,
		"passive_ports":     []int{a.Config.FTPPassiveMin, a.Config.FTPPassiveMax},
		"ftps_required":     true,
		"setup_warning":     "Keep vsftpd disabled until FTPS certificates, firewall rules, and per-site users are configured.",
	})
}

func formatUptime(seconds float64) string {
	days := int(seconds) / (24 * 60 * 60)
	hours := int(seconds) / (60 * 60) % 24
	minutes := int(seconds) / 60 % 60
	if days > 0 {
		return fmt.Sprintf("%dd %02dh", days, hours)
	}
	return fmt.Sprintf("%02dh %02dm", hours, minutes)
}
