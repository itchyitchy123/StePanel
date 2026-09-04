package main

import (
	"strings"
	"testing"
)

func TestValidateConfigRejectsMalformedSafetyLimit(t *testing.T) {
	t.Setenv("STEPANEL_MAX_CONCURRENT_JOBS", "many")
	if err := ValidateConfig(LoadConfig()); err == nil || !strings.Contains(err.Error(), "STEPANEL_MAX_CONCURRENT_JOBS") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestValidateConfigAcceptsOpenLiteSpeed(t *testing.T) {
	cfg := LoadConfig()
	cfg.WebServer = "openlitespeed"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("OpenLiteSpeed config rejected: %v", err)
	}
}

func TestValidateConfigRejectsUnknownWebServer(t *testing.T) {
	cfg := LoadConfig()
	cfg.WebServer = "nginx"
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "STEPANEL_WEBSERVER") {
		t.Fatalf("expected webserver validation error, got %v", err)
	}
}

func TestValidateConfigRequiresDedicatedProductionAuditKey(t *testing.T) {
	t.Setenv("STEPANEL_ENV", "production")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	t.Setenv("STEPANEL_AUDIT_KEY", "12345678901234567890123456789012")
	cfg := LoadConfig()
	cfg.ImportRoot = "/var/lib/ste-panel/imports"
	cfg.BackupRoot = "/var/backups/stepanel"
	cfg.WebRoot = "/var/www"
	cfg.AuditLog = "/var/lib/ste-panel/audit.jsonl"
	cfg.JobState = "/var/lib/ste-panel/jobs.json"
	cfg.RecoveryRoot = "/var/www/sites/.stepanel-recovery"
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("validation error = %v", err)
	}
}
