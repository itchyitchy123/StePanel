package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var pythonVersionPattern = regexp.MustCompile(`^3\.(12|13)$`)
var pythonEntryPattern = regexp.MustCompile(`^[A-Za-z0-9_./:-]{1,160}$`)

type PythonApp struct {
	Site       string `json:"site"`
	Version    string `json:"version"`
	EntryPoint string `json:"entrypoint"`
	Port       int    `json:"port"`
	Workers    int    `json:"workers"`
	Root       string `json:"root"`
	State      string `json:"state"`
}

func (a *App) pythonDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var app PythonApp
	if err := decodeJSON(w, r, 8192, &app); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	app.Site = safeUser(app.Site)
	app.Version = strings.TrimPrefix(strings.TrimSpace(app.Version), "v")
	app.EntryPoint = strings.TrimSpace(app.EntryPoint)
	if app.Site == "" || !pythonVersionPattern.MatchString(app.Version) || !pythonEntryPattern.MatchString(app.EntryPoint) || app.Port < 1024 || app.Port > 65535 || app.Workers < 1 || app.Workers > 64 {
		http.Error(w, "invalid Python application definition", 422)
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
	if err := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "python-apply", app.Site, app.Version, app.Root, app.EntryPoint, strconv.Itoa(app.Port), strconv.Itoa(app.Workers)); err != nil {
		http.Error(w, "Python application helper failed", 502)
		return
	}
	app.State = "running"
	data, _ := json.MarshalIndent(app, "", "  ")
	if err := os.MkdirAll(a.Config.AppRoot, 0750); err != nil || writeAtomic(filepath.Join(a.Config.AppRoot, app.Site+"-python.json"), append(data, '\n'), 0600) != nil {
		http.Error(w, "Python application state could not be saved", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "python.deployed", app.Site, app.EntryPoint)
	writeJSON(w, 202, app)
}

func (a *App) pythonAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/python/"), "/"), "/")
	if len(parts) != 2 || safeUser(parts[0]) == "" || (parts[1] != "start" && parts[1] != "stop" && parts[1] != "restart") {
		http.Error(w, "invalid Python action", 422)
		return
	}
	if !a.canAccessSite(r, parts[0]) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	if err := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, parts[1], parts[0]+"-python"); err != nil {
		http.Error(w, "Python action failed", 502)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "python."+parts[1], parts[0], "systemd action")
	writeJSON(w, 202, map[string]string{"site": parts[0], "action": parts[1]})
}
