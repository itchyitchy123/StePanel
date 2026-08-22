package main

import "os"

type Config struct { Listen, ImportRoot, WebRoot string }
func LoadConfig() Config { c := Config{Listen: ":8080", ImportRoot: "data/imports", WebRoot: "data/www"}; if v := os.Getenv("STEPANEL_LISTEN"); v != "" { c.Listen = v }; if v := os.Getenv("STEPANEL_IMPORT_ROOT"); v != "" { c.ImportRoot = v }; if v := os.Getenv("STEPANEL_WEB_ROOT"); v != "" { c.WebRoot = v }; return c }
