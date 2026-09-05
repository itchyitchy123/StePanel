package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var siteVHostNamePattern = regexp.MustCompile(`^site-[a-z0-9_-]{1,32}-[a-z0-9_-]+\.(conf|caddy)$`)

type siteRoute struct {
	Site   string `json:"site"`
	Domain string `json:"domain"`
}

// siteOverview is the read-only, site-centric view consumed by the dashboard
// and API clients. It deliberately reports paths and states, never credentials
// or environment values.
type siteOverview struct {
	Site          string        `json:"site"`
	DocumentRoot  string        `json:"document_root"`
	Routes        []siteRoute   `json:"routes"`
	Applications  []AppManifest `json:"applications"`
	Proxies       []proxyInfo   `json:"proxies"`
	DatabaseCount int           `json:"database_count"`
	Exists        bool          `json:"exists"`
}

func (a *App) siteOverviewList(w http.ResponseWriter, _ *http.Request) {
	sites := map[string]*siteOverview{}
	entries, _ := os.ReadDir(filepath.Join(a.Config.WebRoot, "sites"))
	for _, entry := range entries {
		if entry.IsDir() && safeUser(entry.Name()) != "" {
			sites[entry.Name()] = newSiteOverview(a.Config, entry.Name())
		}
	}
	for _, app := range managedApps(a.Config.AppRoot) {
		if sites[app.Site] == nil {
			sites[app.Site] = newSiteOverview(a.Config, app.Site)
		}
		sites[app.Site].Applications = append(sites[app.Site].Applications, app)
	}
	if databases, err := managedDatabaseInventory(a.Config); err == nil {
		for _, database := range databases {
			if sites[database.Site] == nil {
				sites[database.Site] = newSiteOverview(a.Config, database.Site)
			}
			sites[database.Site].DatabaseCount++
		}
	}
	result := make([]*siteOverview, 0, len(sites))
	for _, site := range sites {
		site.Routes = siteRoutesFor(a.Config.VHostRoot, site.Site)
		site.Proxies = siteProxiesFor(a.Config.ProxyRoot, site.Site)
		sort.Slice(site.Routes, func(i, j int) bool { return site.Routes[i].Domain < site.Routes[j].Domain })
		sort.Slice(site.Applications, func(i, j int) bool { return site.Applications[i].Domain < site.Applications[j].Domain })
		result = append(result, site)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Site < result[j].Site })
	writeJSON(w, http.StatusOK, map[string]any{"sites": result, "time": time.Now().UTC()})
}

func (a *App) siteOverviewResource(w http.ResponseWriter, r *http.Request) {
	site := strings.TrimPrefix(r.URL.Path, "/api/sites/overview/")
	if site == "" || strings.Contains(site, "/") || safeUser(site) == "" {
		http.Error(w, "invalid site", http.StatusUnprocessableEntity)
		return
	}
	overview := newSiteOverview(a.Config, site)
	overview.Routes = siteRoutesFor(a.Config.VHostRoot, site)
	overview.Proxies = siteProxiesFor(a.Config.ProxyRoot, site)
	for _, app := range managedApps(a.Config.AppRoot) {
		if app.Site == site {
			overview.Applications = append(overview.Applications, app)
		}
	}
	if databases, err := managedDatabaseInventory(a.Config); err == nil {
		for _, database := range databases {
			if database.Site == site {
				overview.DatabaseCount++
			}
		}
	}
	if !overview.Exists && len(overview.Routes) == 0 && len(overview.Applications) == 0 && len(overview.Proxies) == 0 {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func newSiteOverview(cfg Config, site string) *siteOverview {
	root := filepath.Join(cfg.WebRoot, "sites", site, "public")
	_, err := os.Stat(root)
	return &siteOverview{Site: site, DocumentRoot: root, Routes: []siteRoute{}, Applications: []AppManifest{}, Proxies: []proxyInfo{}, Exists: err == nil}
}

func siteRoutesFor(root, site string) []siteRoute {
	entries, _ := os.ReadDir(root)
	routes := []siteRoute{}
	prefix := "site-" + site + "-"
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !siteVHostNamePattern.MatchString(name) || !strings.HasPrefix(name, prefix) {
			continue
		}
		domain := strings.TrimPrefix(name, prefix)
		domain = strings.TrimSuffix(strings.TrimSuffix(domain, ".conf"), ".caddy")
		domain = strings.ReplaceAll(domain, "_", ".")
		routes = append(routes, siteRoute{Site: site, Domain: domain})
	}
	return routes
}

func siteProxiesFor(root, site string) []proxyInfo {
	entries, _ := os.ReadDir(root)
	proxies := []proxyInfo{}
	prefix := site + "-"
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".conf") && !strings.HasSuffix(name, ".caddy") {
			continue
		}
		proxies = append(proxies, proxyInfo{Name: strings.TrimSuffix(strings.TrimSuffix(name, ".conf"), ".caddy"), Config: filepath.Join(root, name)})
	}
	sort.Slice(proxies, func(i, j int) bool { return proxies[i].Name < proxies[j].Name })
	return proxies
}

func managedApps(root string) []AppManifest {
	entries, _ := os.ReadDir(root)
	apps := []AppManifest{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		var app AppManifest
		if err == nil && json.Unmarshal(data, &app) == nil && safeUser(app.Site) != "" {
			apps = append(apps, app)
		}
	}
	return apps
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
	if err := runHelperCommand(r.Context(), a.Config, a.Config.VHostCtl, "apply", input.Site, input.Domain); err != nil {
		http.Error(w, "site helper rejected the route or webserver reload failed", http.StatusServiceUnavailable)
		return
	}
	extension := ".conf"
	if a.Config.WebServer == "caddy" {
		extension = ".caddy"
	}
	name := "site-" + input.Site + "-" + strings.ReplaceAll(input.Domain, ".", "_") + extension
	if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "site.deployed", input.Site, input.Domain); err != nil {
		log.Printf("site deployed but audit persistence is unavailable: %v", err)
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
	if err := runHelperCommand(r.Context(), a.Config, a.Config.VHostCtl, "delete", name); err != nil {
		http.Error(w, "site route was not removed because validation or webserver reload failed", http.StatusServiceUnavailable)
		return
	}
	if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "site.deleted", name, "managed PHP vhost removed"); err != nil {
		log.Printf("site deleted but audit persistence is unavailable: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
}
