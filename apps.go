package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type AppManifest struct {
	Site    string `json:"site"`
	Domain  string `json:"domain"`
	Version string `json:"node_version"`
	Port    int    `json:"port"`
	Root    string `json:"root"`
	State   string `json:"state"`
}

func (a *App) appList(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(a.Config.AppRoot)
	apps := []AppManifest{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(a.Config.AppRoot, entry.Name()))
		if err != nil {
			continue
		}
		var app AppManifest
		if json.Unmarshal(data, &app) == nil {
			apps = append(apps, app)
		}
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Site < apps[j].Site })
	writeJSON(w, http.StatusOK, map[string]any{"apps": apps})
}

func (a *App) appDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	var app AppManifest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&app); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if safeUser(app.Site) == "" || !nodeVersionPattern.MatchString(app.Version) || app.Port < 1024 || app.Port > 65535 {
		http.Error(w, "invalid site, Node version, or port", 422)
		return
	}
	if _, err := localBackend("http://127.0.0.1:" + strconv.Itoa(app.Port)); err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	app.Root = filepath.Join(a.Config.WebRoot, "sites", app.Site, "public")
	if err := ensureInside(a.Config.WebRoot, app.Root); err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	if info, err := os.Stat(app.Root); err != nil || !info.IsDir() {
		http.Error(w, "site document root does not exist", 422)
		return
	}
	app.State = "staged"
	if err := os.MkdirAll(a.Config.AppRoot, 0750); err != nil {
		http.Error(w, "unable to create app state directory", 500)
		return
	}
	data, _ := json.MarshalIndent(app, "", "  ")
	if err := os.WriteFile(filepath.Join(a.Config.AppRoot, app.Site+".json"), append(data, '\n'), 0600); err != nil {
		http.Error(w, "unable to save app manifest", 500)
		return
	}
	if a.Config.AppCtl == "" || exec.Command(a.Config.AppCtl, "apply", app.Site, strings.TrimPrefix(app.Version, "v"), app.Root, strconv.Itoa(app.Port)).Run() != nil {
		http.Error(w, "app manifest saved but systemd helper failed", 503)
		return
	}
	app.State = "running"
	_ = Audit(a.Config.AuditLog, "app.deployed", app.Site, app.Domain+" on port "+strconv.Itoa(app.Port))
	writeJSON(w, http.StatusAccepted, app)
}

func (a *App) appAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/apps/"), "/")
	allowed := map[string]bool{"start": true, "stop": true, "restart": true}
	if len(parts) != 2 || safeUser(parts[0]) == "" || !allowed[parts[1]] {
		http.Error(w, "invalid app action", 422)
		return
	}
	if a.Config.AppCtl == "" || exec.Command(a.Config.AppCtl, parts[1], parts[0]).Run() != nil {
		http.Error(w, "app action failed", 502)
		return
	}
	_ = Audit(a.Config.AuditLog, "app."+parts[1], parts[0], "systemd action")
	writeJSON(w, http.StatusAccepted, map[string]string{"site": parts[0], "action": parts[1]})
}
