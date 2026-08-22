package main

import (
    "encoding/json"
    "html/template"
    "log"
    "net/http"
    "os"
    "strings"
    "time"
)

type App struct { Config Config; View *template.Template; Auth Auth }

func main() {
    cfg := LoadConfig(); if err := os.MkdirAll(cfg.ImportRoot, 0750); err != nil { log.Fatal(err) }
    app := &App{Config: cfg, View: template.Must(template.ParseFiles("web/index.html")), Auth: NewAuth()}
    if !app.Auth.Enabled { log.Println("warning: STEPANEL_ADMIN_PASSWORD is not configured; authentication is disabled") }
    mux := http.NewServeMux(); mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
    mux.HandleFunc("/login", app.Auth.Login); mux.HandleFunc("/logout", app.Auth.Logout)
    mux.Handle("/", app.Auth.Require(http.HandlerFunc(app.dashboard))); mux.HandleFunc("/api/health", app.health)
    mux.Handle("/api/cpmove/inspect", app.Auth.Require(http.HandlerFunc(app.inspect))); mux.Handle("/api/cpmove/import", app.Auth.Require(http.HandlerFunc(app.importBackup))); mux.HandleFunc("/metrics", app.metrics)
    log.Printf("StePanel listening on %s", cfg.Listen); log.Fatal((&http.Server{Addr: cfg.Listen, Handler: logging(mux), ReadHeaderTimeout: 10 * time.Second}).ListenAndServe())
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) { if r.URL.Path != "/" { http.NotFound(w, r); return }; csrf := ""; if cookie, err := r.Cookie("stepanel_csrf"); err == nil { csrf = cookie.Value }; _ = a.View.Execute(w, map[string]any{"Title": "StePanel", "Config": a.Config, "CSRF": csrf, "AuthEnabled": a.Auth.Enabled}) }
func (a *App) health(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, map[string]any{"ok": true, "services": ServiceStatus(), "time": time.Now().UTC()}) }
func (a *App) metrics(w http.ResponseWriter, r *http.Request) { w.Header().Set("Content-Type", "text/plain; version=0.0.4"); _, _ = w.Write([]byte("# HELP stepanel_up StePanel process health\n# TYPE stepanel_up gauge\nstepanel_up 1\n")) }
func (a *App) inspect(w http.ResponseWriter, r *http.Request) { if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }; if !a.Auth.CSRF(r) { http.Error(w, "invalid CSRF token", 403); return }; file, header, err := r.FormFile("backup"); if err != nil { http.Error(w, "backup file is required", 400); return }; defer file.Close(); info, err := InspectCPMove(file, header); if err != nil { http.Error(w, err.Error(), 422); return }; writeJSON(w, 200, info) }
func (a *App) importBackup(w http.ResponseWriter, r *http.Request) { if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }; if !a.Auth.CSRF(r) { http.Error(w, "invalid CSRF token", 403); return }; if err := r.ParseMultipartForm(2 << 30); err != nil { http.Error(w, "invalid upload: "+err.Error(), 400); return }; if r.FormValue("confirm") != "IMPORT" { http.Error(w, "type IMPORT to authorize restore", 400); return }; file, header, err := r.FormFile("backup"); if err != nil { http.Error(w, "backup file is required", 400); return }; defer file.Close(); user := safeUser(r.FormValue("username")); if user == "" { http.Error(w, "a valid account username is required", 400); return }; result, err := RestoreCPMove(a.Config, file, header, user, r.FormValue("restore_databases") == "on"); if err != nil { _ = Audit(a.Config.AuditLog, "cpmove.restore.failed", user, err.Error()); http.Error(w, err.Error(), 422); return }; _ = Audit(a.Config.AuditLog, "cpmove.restore.completed", user, result.StagedAt); writeJSON(w, 200, result) }
func logging(next http.Handler) http.Handler { return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { started := time.Now(); w.Header().Set("X-Content-Type-Options", "nosniff"); w.Header().Set("X-Frame-Options", "DENY"); w.Header().Set("Referrer-Policy", "same-origin"); next.ServeHTTP(w, r); log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started)) }) }
func writeJSON(w http.ResponseWriter, status int, value any) { w.Header().Set("Content-Type", "application/json"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(value) }
func safeUser(value string) string { value = strings.TrimSpace(value); if len(value) > 32 || value == "" { return "" }; for _, r := range value { if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') { return "" } }; return value }
