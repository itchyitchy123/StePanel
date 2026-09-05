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

func TestValidateConfigAcceptsCaddy(t *testing.T) {
	cfg := LoadConfig()
	cfg.WebServer = "caddy"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("Caddy config rejected: %v", err)
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

func TestValidateConfigRequiresLoopbackOrTLSInProduction(t *testing.T) {
	t.Setenv("STEPANEL_ENV", "production")
	t.Setenv("STEPANEL_AUDIT_KEY", "audit-key-that-is-long-enough-123456")
	t.Setenv("STEPANEL_SESSION_SECRET", "session-key-that-is-long-enough-123456")
	cfg := LoadConfig()
	cfg.Listen = ":8080"
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected production listener validation error, got %v", err)
	}
}

func TestValidateConfigAcceptsProductionTLSPaths(t *testing.T) {
	t.Setenv("STEPANEL_ENV", "production")
	t.Setenv("STEPANEL_AUDIT_KEY", "audit-key-that-is-long-enough-123456")
	t.Setenv("STEPANEL_SESSION_SECRET", "session-key-that-is-long-enough-123456")
	cfg := LoadConfig()
	cfg.TLSCertFile = "/etc/stepanel/cert.pem"
	cfg.TLSKeyFile = "/etc/stepanel/key.pem"
	cfg.ImportRoot = "/var/lib/ste-panel/imports"
	cfg.BackupRoot = "/var/backups/stepanel"
	cfg.WebRoot = "/var/www"
	cfg.MailRoot = "/var/lib/ste-panel/mail"
	cfg.NVMDir = "/var/lib/ste-panel/nvm"
	cfg.ProxyRoot = "/var/lib/ste-panel/proxy"
	cfg.VHostRoot = "/var/lib/ste-panel/vhosts"
	cfg.AppRoot = "/var/lib/ste-panel/apps"
	cfg.MalwareRoot = "/var/lib/ste-panel/quarantine"
	cfg.AuditLog = "/var/lib/ste-panel/audit.jsonl"
	cfg.JobState = "/var/lib/ste-panel/jobs.json"
	cfg.SessionState = "/var/lib/ste-panel/sessions.json"
	cfg.RecoveryRoot = "/var/www/sites/.stepanel-recovery"
	cfg.WPressExtract = "/usr/local/bin/wpress-extract"
	cfg.WPCLI = "/usr/local/bin/wp"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("TLS production config rejected: %v", err)
	}
}

func TestValidateConfigAcceptsTrustedTLSTermination(t *testing.T) {
	t.Setenv("STEPANEL_ENV", "production")
	t.Setenv("STEPANEL_TLS_TERMINATED", "1")
	t.Setenv("STEPANEL_AUDIT_KEY", "audit-key-that-is-long-enough-123456")
	t.Setenv("STEPANEL_SESSION_SECRET", "session-key-that-is-long-enough-123456")
	cfg := LoadConfig()
	cfg.Listen = ":8080"
	cfg.ImportRoot = "/var/lib/ste-panel/imports"
	cfg.BackupRoot = "/var/lib/ste-panel/backups"
	cfg.WebRoot = "/var/www"
	cfg.MailRoot = "/var/lib/ste-panel/mail"
	cfg.NVMDir = "/var/lib/ste-panel/nvm"
	cfg.ProxyRoot = "/var/lib/ste-panel/proxy"
	cfg.VHostRoot = "/var/lib/ste-panel/vhosts"
	cfg.AppRoot = "/var/lib/ste-panel/apps"
	cfg.MalwareRoot = "/var/lib/ste-panel/quarantine"
	cfg.AuditLog = "/var/lib/ste-panel/audit.jsonl"
	cfg.JobState = "/var/lib/ste-panel/jobs.json"
	cfg.SessionState = "/var/lib/ste-panel/sessions.json"
	cfg.RecoveryRoot = "/var/www/sites/.stepanel-recovery"
	cfg.WPressExtract = "/usr/local/bin/wpress-extract"
	cfg.WPCLI = "/usr/local/bin/wp"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("trusted proxy TLS config rejected: %v", err)
	}
}

func TestValidateConfigRejectsInvalidTLSTerminationFlag(t *testing.T) {
	t.Setenv("STEPANEL_ENV", "production")
	t.Setenv("STEPANEL_TLS_TERMINATED", "yes")
	t.Setenv("STEPANEL_AUDIT_KEY", "audit-key-that-is-long-enough-123456")
	t.Setenv("STEPANEL_SESSION_SECRET", "session-key-that-is-long-enough-123456")
	if err := ValidateConfig(LoadConfig()); err == nil || !strings.Contains(err.Error(), "STEPANEL_TLS_TERMINATED") {
		t.Fatalf("expected TLS termination validation error, got %v", err)
	}
}

func TestValidateConfigRequiresOffsiteBackupTarget(t *testing.T) {
	t.Setenv("STEPANEL_REQUIRE_OFFSITE_BACKUP", "1")
	if err := ValidateConfig(LoadConfig()); err == nil || !strings.Contains(err.Error(), "STEPANEL_OFFSITE_TARGET") {
		t.Fatalf("expected offsite target validation error, got %v", err)
	}
}

func TestValidateConfigAcceptsPostgreSQLAndAdminPath(t *testing.T) {
	t.Setenv("STEPANEL_DB_ENGINE", "postgresql")
	t.Setenv("STEPANEL_DB_VERSION", "16")
	t.Setenv("STEPANEL_DB_ADMIN_URL", "/database/postgres")
	cfg := LoadConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("PostgreSQL config rejected: %v", err)
	}
	if cfg.DBAdminURL != "/database/postgres" {
		t.Fatalf("DBAdminURL = %q", cfg.DBAdminURL)
	}
}

func TestValidateConfigRejectsUnsafeDatabaseAdminPath(t *testing.T) {
	cfg := LoadConfig()
	cfg.DBAdminURL = "https://admin.example.test"
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "STEPANEL_DB_ADMIN_URL") {
		t.Fatalf("expected database admin URL validation error, got %v", err)
	}
}
