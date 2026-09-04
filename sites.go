package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var siteVHostNamePattern = regexp.MustCompile(`^site-[a-z0-9_-]{1,32}-[a-z0-9_-]+\.conf$`)

type siteRoute struct {
	Site   string `json:"site"`
	Domain string `json:"domain"`
}

func (a *App) siteList(w http.ResponseWriter, _ *http.Request) {
	entries, err := os.ReadDir(a.Config.VHostRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, "unable to inspect managed site routes", http.StatusInternalServerError)
		return
	}
	routes := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && siteVHostNamePattern.MatchString(entry.Name()) {
			routes = append(routes, entry.Name())
		}
	}
	sort.Strings(routes)
	writeJSON(w, http.StatusOK, map[string]any{"sites": routes})
}

func (a *App) siteDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	var input siteRoute
	if err := decodeJSON(w, r, 4096, &input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	input.Domain = strings.ToLower(strings.TrimSpace(input.Domain))
	if safeUser(input.Site) == "" || len(input.Domain)+len(input.Site) > 220 || !domainPattern.MatchString(input.Domain) {
		http.Error(w, "invalid site or domain", http.StatusUnprocessableEntity)
		return
	}
	publicRoot := filepath.Join(a.Config.WebRoot, "sites", input.Site, "public")
	if err := ensureInside(a.Config.WebRoot, publicRoot); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if info, err := os.Stat(publicRoot); err != nil || !info.IsDir() {
		http.Error(w, "site document root does not exist", http.StatusUnprocessableEntity)
		return
	}
	if a.Config.VHostCtl == "" || helperCommand(a.Config, a.Config.VHostCtl, "apply", input.Site, input.Domain).Run() != nil {
		http.Error(w, "site helper rejected the route or webserver reload failed", http.StatusServiceUnavailable)
		return
	}
	name := "site-" + input.Site + "-" + strings.ReplaceAll(input.Domain, ".", "_") + ".conf"
	if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "site.deployed", input.Site, input.Domain); err != nil {
		http.Error(w, "site deployed but audit persistence is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"site": input.Site, "domain": input.Domain, "config": filepath.Join(a.Config.VHostRoot, name)})
}

func (a *App) siteManage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/sites/")
	if !siteVHostNamePattern.MatchString(name) {
		http.Error(w, "invalid site route", http.StatusUnprocessableEntity)
		return
	}
	path := filepath.Join(a.Config.VHostRoot, name)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "site route not found", http.StatusNotFound)
		return
	}
	if a.Config.VHostCtl == "" || helperCommand(a.Config, a.Config.VHostCtl, "delete", name).Run() != nil {
		http.Error(w, "site route was not removed because validation or webserver reload failed", http.StatusServiceUnavailable)
		return
	}
	if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "site.deleted", name, "managed PHP vhost removed"); err != nil {
		http.Error(w, "site deleted but audit persistence is unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
}
