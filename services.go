package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type ServiceSummary struct {
	Name   string
	Region string
	Status string
	Load   string
	Uptime string
}

func ServiceStatus() map[string]string {
	result := map[string]string{}
	for _, service := range []string{"apache2", "httpd", "mysql", "mariadb", "php-fpm", "fail2ban", "fpm-lens", "exim4", "exim", "dovecot", "spamassassin", "spamd"} {
		if _, err := exec.LookPath(service); err == nil {
			result[service] = "installed"
		}
	}
	for _, apache := range []string{"apachectl", "httpd"} {
		if _, err := exec.LookPath(apache); err != nil {
			continue
		}
		if output, err := exec.Command(apache, "-M").CombinedOutput(); err == nil && (bytes.Contains(output, []byte("security2_module")) || bytes.Contains(output, []byte("security3_module"))) {
			result["modsecurity"] = "enabled"
			break
		}
	}
	return result
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
	services := []string{"apache2", "mysql", "php-fpm", "fail2ban", "modsecurity", "exim", "dovecot", "spamassassin"}
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

func formatUptime(seconds float64) string {
	days := int(seconds) / (24 * 60 * 60)
	hours := int(seconds) / (60 * 60) % 24
	minutes := int(seconds) / 60 % 60
	if days > 0 {
		return fmt.Sprintf("%dd %02dh", days, hours)
	}
	return fmt.Sprintf("%02dh %02dm", hours, minutes)
}
