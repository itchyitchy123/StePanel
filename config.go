package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Listen, ImportRoot, BackupRoot, WebRoot, MailRoot, NVMDir                  string
	ProxyRoot, VHostRoot, AppRoot, MalwareRoot, AppCtl, ProxyCtl               string
	SiteCtl, VHostCtl, Certbot, DBCtl                                          string
	WPressExtract, WPCLI, AuditLog, JobState, SessionState, RecoveryRoot, Sudo string
	DBHost, DBUser, DBPassword                                                 string
	Production                                                                 bool
	MaxUpload                                                                  int64
	MaxEntries, MaxConcurrentJobs, StageRetentionHours                         int
	FTPPassiveMin, FTPPassiveMax                                               int
	MinFreeBytes                                                               uint64
}

func LoadConfig() Config {
	c := Config{Listen: ":8080", ImportRoot: "data/imports", BackupRoot: "data/backups", WebRoot: "data/www", MailRoot: "data/mail", NVMDir: "data/nvm", ProxyRoot: "data/proxy", VHostRoot: "data/vhosts", AppRoot: "data/apps", MalwareRoot: "data/quarantine", AppCtl: "/usr/local/sbin/stepanel-appctl", ProxyCtl: "/usr/local/sbin/stepanel-proxyctl", VHostCtl: "/usr/local/sbin/stepanel-vhostctl", Certbot: "/usr/local/sbin/stepanel-certbot", WPressExtract: "wpress-extract", WPCLI: "wp", AuditLog: "data/stepanel-audit.jsonl", JobState: "data/jobs.json", SessionState: "data/sessions.json", RecoveryRoot: "data/www/sites/.stepanel-recovery", MaxUpload: 20 << 30, MaxEntries: 1000000, MaxConcurrentJobs: 2, StageRetentionHours: 168, MinFreeBytes: 1 << 30, FTPPassiveMin: 40100, FTPPassiveMax: 40200}
	if v := os.Getenv("STEPANEL_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("STEPANEL_IMPORT_ROOT"); v != "" {
		c.ImportRoot = v
	}
	if v := os.Getenv("STEPANEL_BACKUP_ROOT"); v != "" {
		c.BackupRoot = v
	}
	if v := os.Getenv("STEPANEL_WEB_ROOT"); v != "" {
		c.WebRoot = v
	}
	if v := os.Getenv("STEPANEL_MAIL_ROOT"); v != "" {
		c.MailRoot = v
	}
	if v := os.Getenv("STEPANEL_NVM_DIR"); v != "" {
		c.NVMDir = v
	}
	if v := os.Getenv("STEPANEL_PROXY_ROOT"); v != "" {
		c.ProxyRoot = v
	}
	if v := os.Getenv("STEPANEL_VHOST_ROOT"); v != "" {
		c.VHostRoot = v
	}
	if v := os.Getenv("STEPANEL_APP_ROOT"); v != "" {
		c.AppRoot = v
	}
	if v := os.Getenv("STEPANEL_APPCTL"); v != "" {
		c.AppCtl = v
	}
	if v := os.Getenv("STEPANEL_PROXYCTL"); v != "" {
		c.ProxyCtl = v
	}
	if v := os.Getenv("STEPANEL_SITECTL"); v != "" {
		c.SiteCtl = v
	}
	if v := os.Getenv("STEPANEL_VHOSTCTL"); v != "" {
		c.VHostCtl = v
	}
	if v := os.Getenv("STEPANEL_DBCTL"); v != "" {
		c.DBCtl = v
	}
	if v := os.Getenv("STEPANEL_CERTBOT"); v != "" {
		c.Certbot = v
	}
	if v := os.Getenv("STEPANEL_WPRESS_EXTRACT"); v != "" {
		c.WPressExtract = v
	}
	if v := os.Getenv("STEPANEL_WPCLI"); v != "" {
		c.WPCLI = v
	}
	if v := os.Getenv("STEPANEL_MALWARE_ROOT"); v != "" {
		c.MalwareRoot = v
	}
	if v := os.Getenv("STEPANEL_AUDIT_LOG"); v != "" {
		c.AuditLog = v
	}
	if v := os.Getenv("STEPANEL_JOB_STATE"); v != "" {
		c.JobState = v
	}
	if v := os.Getenv("STEPANEL_SESSION_STATE"); v != "" {
		c.SessionState = v
	}
	if v := os.Getenv("STEPANEL_RECOVERY_ROOT"); v != "" {
		c.RecoveryRoot = v
	}
	if v := os.Getenv("STEPANEL_SUDO"); v != "" {
		c.Sudo = v
	}
	c.DBHost = os.Getenv("STEPANEL_DB_HOST")
	c.DBUser = os.Getenv("STEPANEL_DB_USER")
	c.DBPassword = os.Getenv("STEPANEL_DB_PASSWORD")
	c.Production = os.Getenv("STEPANEL_ENV") == "production"
	if v, err := strconv.ParseInt(os.Getenv("STEPANEL_MAX_UPLOAD_BYTES"), 10, 64); err == nil && v > 0 && v <= 20<<30 {
		c.MaxUpload = v
	}
	if v, err := strconv.Atoi(os.Getenv("STEPANEL_MAX_ARCHIVE_ENTRIES")); err == nil && v > 0 && v <= 1000000 {
		c.MaxEntries = v
	}
	if v, err := strconv.Atoi(os.Getenv("STEPANEL_MAX_CONCURRENT_JOBS")); err == nil && v > 0 && v <= 32 {
		c.MaxConcurrentJobs = v
	}
	if v, err := strconv.Atoi(os.Getenv("STEPANEL_STAGE_RETENTION_HOURS")); err == nil && v > 0 {
		c.StageRetentionHours = v
	}
	if v, err := strconv.ParseUint(os.Getenv("STEPANEL_MIN_FREE_BYTES"), 10, 64); err == nil && v > 0 {
		c.MinFreeBytes = v
	}
	if v, err := strconv.Atoi(os.Getenv("STEPANEL_FTP_PASSIVE_MIN")); err == nil && v >= 1024 && v <= 65534 {
		c.FTPPassiveMin = v
	}
	if v, err := strconv.Atoi(os.Getenv("STEPANEL_FTP_PASSIVE_MAX")); err == nil && v > c.FTPPassiveMin && v <= 65535 {
		c.FTPPassiveMax = v
	}
	return c
}

// ValidateConfig rejects unsafe or malformed settings instead of allowing a
// typo to silently select a default. LoadConfig remains deliberately simple so
// callers that construct Config values directly (including tests and tools) do
// not need an error-returning configuration API.
func ValidateConfig(c Config) error {
	var problems []error
	switch environment := os.Getenv("STEPANEL_ENV"); environment {
	case "", "development", "lab", "test", "production":
	default:
		problems = append(problems, fmt.Errorf("STEPANEL_ENV %q is invalid; use development, lab, test, or production", environment))
	}
	if host, port, err := net.SplitHostPort(c.Listen); err != nil {
		problems = append(problems, fmt.Errorf("STEPANEL_LISTEN: %w", err))
	} else if value, err := strconv.Atoi(port); err != nil || value < 1 || value > 65535 {
		problems = append(problems, errors.New("STEPANEL_LISTEN must contain a port from 1 to 65535"))
	} else if strings.ContainsAny(host, "\r\n") {
		problems = append(problems, errors.New("STEPANEL_LISTEN contains invalid characters"))
	}

	validateIntegerEnvironment(&problems, "STEPANEL_MAX_UPLOAD_BYTES", 1, 20<<30)
	validateIntegerEnvironment(&problems, "STEPANEL_MAX_ARCHIVE_ENTRIES", 1, 1_000_000)
	validateIntegerEnvironment(&problems, "STEPANEL_MAX_CONCURRENT_JOBS", 1, 32)
	validateIntegerEnvironment(&problems, "STEPANEL_STAGE_RETENTION_HOURS", 1, 87_600)
	validateIntegerEnvironment(&problems, "STEPANEL_MIN_FREE_BYTES", 1, int64(^uint64(0)>>1))
	validateIntegerEnvironment(&problems, "STEPANEL_FTP_PASSIVE_MIN", 1024, 65534)
	validateIntegerEnvironment(&problems, "STEPANEL_FTP_PASSIVE_MAX", 1025, 65535)
	if c.MaxUpload < 1 || c.MaxUpload > 20<<30 || c.MaxEntries < 1 || c.MaxEntries > 1_000_000 || c.MaxConcurrentJobs < 1 || c.MaxConcurrentJobs > 32 || c.StageRetentionHours < 1 || c.StageRetentionHours > 87_600 || c.MinFreeBytes < 1 {
		problems = append(problems, errors.New("configured resource limits are outside their supported ranges"))
	}
	if c.FTPPassiveMax <= c.FTPPassiveMin {
		problems = append(problems, errors.New("STEPANEL_FTP_PASSIVE_MAX must be greater than STEPANEL_FTP_PASSIVE_MIN"))
	}

	paths := map[string]string{
		"STEPANEL_IMPORT_ROOT":   c.ImportRoot,
		"STEPANEL_BACKUP_ROOT":   c.BackupRoot,
		"STEPANEL_WEB_ROOT":      c.WebRoot,
		"STEPANEL_MAIL_ROOT":     c.MailRoot,
		"STEPANEL_NVM_DIR":       c.NVMDir,
		"STEPANEL_PROXY_ROOT":    c.ProxyRoot,
		"STEPANEL_VHOST_ROOT":    c.VHostRoot,
		"STEPANEL_APP_ROOT":      c.AppRoot,
		"STEPANEL_MALWARE_ROOT":  c.MalwareRoot,
		"STEPANEL_AUDIT_LOG":     c.AuditLog,
		"STEPANEL_JOB_STATE":     c.JobState,
		"STEPANEL_SESSION_STATE": c.SessionState,
		"STEPANEL_RECOVERY_ROOT": c.RecoveryRoot,
	}
	for name, path := range paths {
		if strings.TrimSpace(path) == "" || strings.ContainsAny(path, "\x00\r\n") {
			problems = append(problems, fmt.Errorf("%s must be a non-empty filesystem path", name))
		} else if c.Production && !filepath.IsAbs(path) {
			problems = append(problems, fmt.Errorf("%s must be absolute in production", name))
		} else if c.Production && filepath.Clean(path) == string(os.PathSeparator) {
			problems = append(problems, fmt.Errorf("%s must not be the filesystem root", name))
		}
	}
	if c.Production {
		for name, path := range map[string]string{"STEPANEL_APPCTL": c.AppCtl, "STEPANEL_PROXYCTL": c.ProxyCtl, "STEPANEL_SITECTL": c.SiteCtl, "STEPANEL_VHOSTCTL": c.VHostCtl, "STEPANEL_DBCTL": c.DBCtl, "STEPANEL_CERTBOT": c.Certbot, "STEPANEL_SUDO": c.Sudo} {
			if path != "" && (!filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n")) {
				problems = append(problems, fmt.Errorf("%s must be an absolute executable path in production", name))
			}
		}
	}
	if c.Production {
		key := strings.TrimSpace(os.Getenv("STEPANEL_AUDIT_KEY"))
		if key == "" {
			if data, err := os.ReadFile(auditKeyPath); err == nil {
				key = strings.TrimSpace(string(data))
			}
		}
		if len(key) < 32 {
			problems = append(problems, errors.New("production requires a dedicated STEPANEL_AUDIT_KEY of at least 32 characters"))
		}
		if strings.ContainsAny(os.Getenv("STEPANEL_AUDIT_KEY"), "\r\n") {
			problems = append(problems, errors.New("STEPANEL_AUDIT_KEY must not contain newlines"))
		}
		if key != "" && key == os.Getenv("STEPANEL_SESSION_SECRET") {
			problems = append(problems, errors.New("STEPANEL_AUDIT_KEY must differ from STEPANEL_SESSION_SECRET"))
		}
	}
	return errors.Join(problems...)
}

func validateIntegerEnvironment(problems *[]error, name string, minimum, maximum int64) {
	raw := os.Getenv(name)
	if raw == "" {
		return
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < minimum || value > maximum {
		*problems = append(*problems, fmt.Errorf("%s must be an integer from %d to %d", name, minimum, maximum))
	}
}
