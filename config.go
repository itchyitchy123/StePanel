package main

import (
	"os"
	"strconv"
)

type Config struct {
	Listen, ImportRoot, WebRoot, MailRoot, NVMDir                 string
	ProxyRoot, VHostRoot, AppRoot, MalwareRoot, AppCtl, ProxyCtl  string
	SiteCtl, VHostCtl, Certbot, DBCtl                             string
	WPressExtract, WPCLI, AuditLog, JobState, RecoveryRoot, Sudo  string
	DBHost, DBUser, DBPassword                                    string
	Production                                                    bool
	MaxUpload                                                     int64
	MaxEntries, StageRetentionHours, FTPPassiveMin, FTPPassiveMax int
	MinFreeBytes                                                  uint64
}

func LoadConfig() Config {
	c := Config{Listen: ":8080", ImportRoot: "data/imports", WebRoot: "data/www", MailRoot: "data/mail", NVMDir: "data/nvm", ProxyRoot: "data/proxy", VHostRoot: "data/vhosts", AppRoot: "data/apps", MalwareRoot: "data/quarantine", AppCtl: "/usr/local/sbin/stepanel-appctl", ProxyCtl: "/usr/local/sbin/stepanel-proxyctl", VHostCtl: "/usr/local/sbin/stepanel-vhostctl", Certbot: "/usr/local/sbin/stepanel-certbot", WPressExtract: "wpress-extract", WPCLI: "wp", AuditLog: "data/stepanel-audit.jsonl", JobState: "data/jobs.json", RecoveryRoot: "data/www/sites/.stepanel-recovery", MaxUpload: 20 << 30, MaxEntries: 1000000, StageRetentionHours: 168, MinFreeBytes: 1 << 30, FTPPassiveMin: 40100, FTPPassiveMax: 40200}
	if v := os.Getenv("STEPANEL_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("STEPANEL_IMPORT_ROOT"); v != "" {
		c.ImportRoot = v
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
