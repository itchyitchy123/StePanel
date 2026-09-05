package main

import (
	"net/http"
	"os"
)

type DatabaseAdmin struct {
	Engine       string `json:"engine"`
	Version      string `json:"version"`
	Host         string `json:"host"`
	Service      string `json:"service"`
	Status       string `json:"status"`
	Client       string `json:"client"`
	AdminProduct string `json:"admin_product"`
	AdminURL     string `json:"admin_url"`
	AdminReady   bool   `json:"admin_ready"`
	Local        bool   `json:"local"`
}

func (a *App) database(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"database": a.DatabaseAdmin()})
}

func (a *App) DatabaseAdmin() DatabaseAdmin {
	cfg := a.Config
	engine := cfg.DBEngine
	if engine == "" {
		engine = "mysql"
	}
	product, url, paths := "phpMyAdmin", "/phpmyadmin", []string{"/usr/share/phpmyadmin", "/usr/share/phpMyAdmin"}
	service, client := "mysql", "mariadb"
	if engine == "postgresql" {
		product, url, paths = "phpPgAdmin", "/phppgadmin", []string{"/usr/share/phppgadmin", "/usr/share/phpPgAdmin"}
		service, client = "postgresql", "psql"
	} else if engine == "mariadb" {
		service = "mariadb"
	}
	if cfg.DBAdminURL != "" {
		url = cfg.DBAdminURL
	}
	status := ServiceStatus()[service]
	if status == "" && engine == "mysql" {
		status = ServiceStatus()["mariadb"]
	}
	if status == "" {
		status = "missing"
	}
	adminReady := false
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			adminReady = true
			break
		}
	}
	// Distribution packages provide the Apache alias/configuration. Other
	// webservers need an explicit operator-managed PHP route, so do not expose
	// a dashboard link that would silently lead to a directory or 404.
	adminReady = adminReady && cfg.WebServer == "apache"
	return DatabaseAdmin{Engine: engine, Version: cfg.DBVersion, Host: databaseHost(cfg), Service: service, Status: status, Client: client, AdminProduct: product, AdminURL: url, AdminReady: adminReady, Local: cfg.DBHost == "" || cfg.DBHost == "localhost" || cfg.DBHost == "127.0.0.1"}
}

func databaseHost(cfg Config) string {
	if cfg.DBHost == "" {
		return "local socket"
	}
	return cfg.DBHost
}
