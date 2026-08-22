package main

import "os"

type Config struct { Listen, ImportRoot, WebRoot, AuditLog string }
func LoadConfig() Config { c := Config{Listen: ":8080", ImportRoot: "data/imports", WebRoot: "data/www", AuditLog: "data/stepanel-audit.jsonl"}; if v := os.Getenv("STEPANEL_LISTEN"); v != "" { c.Listen = v }; if v := os.Getenv("STEPANEL_IMPORT_ROOT"); v != "" { c.ImportRoot = v }; if v := os.Getenv("STEPANEL_WEB_ROOT"); v != "" { c.WebRoot = v }; if v := os.Getenv("STEPANEL_AUDIT_LOG"); v != "" { c.AuditLog = v }; return c }
