package main

import "os"

type Config struct {
	Listen, ImportRoot, WebRoot, MailRoot, NVMDir, ProxyRoot, ApacheReload, AuditLog string
	DBHost, DBUser, DBPassword                                                       string
	Production                                                                       bool
	MaxUpload                                                                        int64
	MaxEntries                                                                       int
}

func LoadConfig() Config {
	c := Config{Listen: ":8080", ImportRoot: "data/imports", WebRoot: "data/www", MailRoot: "data/mail", NVMDir: "data/nvm", ProxyRoot: "data/proxy", ApacheReload: "/usr/local/sbin/stepanel-apache-reload", AuditLog: "data/stepanel-audit.jsonl", MaxUpload: 20 << 30, MaxEntries: 1000000}
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
	if v := os.Getenv("STEPANEL_APACHE_RELOAD"); v != "" {
		c.ApacheReload = v
	}
	if v := os.Getenv("STEPANEL_AUDIT_LOG"); v != "" {
		c.AuditLog = v
	}
	c.DBHost = os.Getenv("STEPANEL_DB_HOST")
	c.DBUser = os.Getenv("STEPANEL_DB_USER")
	c.DBPassword = os.Getenv("STEPANEL_DB_PASSWORD")
	c.Production = os.Getenv("STEPANEL_ENV") == "production"
	return c
}
