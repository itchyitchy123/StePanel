package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type App struct {
	Config  Config
	View    *template.Template
	Auth    Auth
	Jobs    *Jobs
	Metrics *Metrics
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "hash-password" {
		password, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
		if err != nil || len(password) == 0 || len(password) > 1024 || strings.ContainsAny(string(password), "\r\n") {
			log.Fatal("password must be 1-1024 bytes and contain no newlines")
		}
		hash, err := hashPassword(string(password))
		if err != nil {
			log.Fatal(err)
		}
		_, _ = fmt.Fprintln(os.Stdout, hash)
		return
	}
	cfg := LoadConfig()
	if cfg.DBCtl != "" {
		if output, err := helperCommand(cfg, cfg.DBCtl, "reconcile").CombinedOutput(); err != nil {
			log.Fatalf("reconcile interrupted database operations: %v: %s", err, strings.TrimSpace(string(output)))
		}
	}
	if err := os.MkdirAll(cfg.ImportRoot, 0750); err != nil {
		log.Fatal(err)
	}
	databaseRecoveries, err := RecoverTransactionDatabases(cfg, cfg.RecoveryRoot)
	if err != nil {
		log.Fatalf("recover interrupted database transactions: %v", err)
	}
	for _, id := range databaseRecoveries {
		log.Printf("recovered databases for interrupted site transaction %s", id)
		_ = Audit(cfg.AuditLog, "restore.database-recovered", id, "managed databases removed after unclean shutdown")
	}
	recovered, err := RecoverSiteTransactions(cfg.RecoveryRoot)
	if err != nil {
		log.Fatalf("recover interrupted site transactions: %v", err)
	}
	for _, id := range recovered {
		txn, loadErr := loadSiteTransaction(filepath.Join(cfg.RecoveryRoot, id))
		if loadErr != nil {
			log.Fatalf("load recovered site transaction %s: %v", id, loadErr)
		}
		if sealErr := siteHelper(cfg, "seal", txn.Site); sealErr != nil {
			log.Fatalf("seal recovered site transaction %s: %v", id, sealErr)
		}
		log.Printf("recovered interrupted site transaction %s", id)
		_ = Audit(cfg.AuditLog, "restore.recovered", id, "previous site restored after unclean shutdown")
	}
	_ = CleanupImportStages(cfg.ImportRoot, time.Duration(cfg.StageRetentionHours)*time.Hour)
	_ = CleanupSiteTransactions(cfg.RecoveryRoot, time.Duration(cfg.StageRetentionHours)*time.Hour)
	auth, err := NewAuth(cfg.Production)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Production && !auth.Enabled {
		log.Fatal("authentication must be configured in production")
	}
	view := template.Must(template.New("index.html").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).ParseFiles("web/index.html"))
	jobs, err := OpenJobs(cfg.JobState)
	if err != nil {
		log.Fatalf("open persistent job state: %v", err)
	}
	app := &App{Config: cfg, View: view, Auth: auth, Jobs: jobs, Metrics: NewMetrics()}
	if !app.Auth.Enabled {
		log.Println("warning: authentication is disabled; set STEPANEL_ADMIN_PASSWORD and STEPANEL_SESSION_SECRET")
	}
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			app.Jobs.Cleanup(24 * time.Hour)
			if err := CleanupImportStages(app.Config.ImportRoot, time.Duration(app.Config.StageRetentionHours)*time.Hour); err != nil {
				log.Printf("import stage cleanup: %v", err)
			}
			if err := CleanupSiteTransactions(app.Config.RecoveryRoot, time.Duration(app.Config.StageRetentionHours)*time.Hour); err != nil {
				log.Printf("site recovery cleanup: %v", err)
			}
		}
	}()
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	mux.HandleFunc("/login", app.Auth.Login)
	mux.HandleFunc("/logout", app.Auth.Logout)
	mux.Handle("/", app.Auth.Require(http.HandlerFunc(app.dashboard)))
	mux.HandleFunc("/api/health", app.health)
	mux.Handle("/api/services", app.Auth.Require(http.HandlerFunc(app.services)))
	mux.Handle("/api/ftp", app.Auth.Require(http.HandlerFunc(app.ftpStatus)))
	mux.Handle("/api/security/audit", app.Auth.Require(http.HandlerFunc(app.securityAudit)))
	mux.Handle("/api/security/scan", app.Auth.Require(http.HandlerFunc(app.malwareScan)))
	mux.Handle("/api/certificates/issue", app.Auth.Require(http.HandlerFunc(app.issueCertificate)))
	mux.Handle("/api/node/versions", app.Auth.Require(http.HandlerFunc(app.nodeVersions)))
	mux.Handle("/api/node/select", app.Auth.Require(http.HandlerFunc(app.selectNode)))
	mux.Handle("/api/proxy/deploy", app.Auth.Require(http.HandlerFunc(app.deployProxy)))
	mux.Handle("/api/proxy", app.Auth.Require(http.HandlerFunc(app.proxyList)))
	mux.Handle("/api/proxy/test", app.Auth.Require(http.HandlerFunc(app.proxyTest)))
	mux.Handle("/api/proxy/", app.Auth.Require(http.HandlerFunc(app.proxyManage)))
	mux.Handle("/api/sites", app.Auth.Require(http.HandlerFunc(app.siteList)))
	mux.Handle("/api/sites/deploy", app.Auth.Require(http.HandlerFunc(app.siteDeploy)))
	mux.Handle("/api/sites/", app.Auth.Require(http.HandlerFunc(app.siteManage)))
	mux.Handle("/api/apps", app.Auth.Require(http.HandlerFunc(app.appList)))
	mux.Handle("/api/apps/deploy", app.Auth.Require(http.HandlerFunc(app.appDeploy)))
	mux.Handle("/api/apps/", app.Auth.Require(http.HandlerFunc(app.appAction)))
	mux.Handle("/api/cpmove/inspect", app.Auth.Require(http.HandlerFunc(app.inspect)))
	mux.Handle("/api/cpmove/import", app.Auth.Require(http.HandlerFunc(app.importBackup)))
	mux.Handle("/api/wpress/preflight", app.Auth.Require(http.HandlerFunc(app.wpressPreflight)))
	mux.Handle("/api/wpress/import", app.Auth.Require(http.HandlerFunc(app.wpressImport)))
	mux.Handle("/api/jobs/", app.Auth.Require(http.HandlerFunc(app.jobStatus)))
	metricsHandler := http.Handler(http.HandlerFunc(app.metrics))
	if os.Getenv("STEPANEL_METRICS_PUBLIC") != "1" {
		metricsHandler = app.Auth.Require(metricsHandler)
	}
	mux.Handle("/metrics", metricsHandler)
	server := &http.Server{Addr: cfg.Listen, Handler: logging(mux, app.Metrics), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("StePanel listening on %s", cfg.Listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown: %v", err)
	}
	jobCtx, jobCancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer jobCancel()
	if err := app.Jobs.Wait(jobCtx); err != nil {
		log.Printf("timed out waiting for active jobs: %v", err)
	}
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	csrf := ""
	if cookie, err := r.Cookie("stepanel_csrf"); err == nil {
		csrf = cookie.Value
	}
	_ = a.View.Execute(w, map[string]any{"Title": "StePanel", "Config": a.Config, "CSRF": csrf, "AuthEnabled": a.Auth.Enabled, "Servers": ServiceSummaries(), "Security": a.SecurityChecks()})
}
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{"ok": true, "version": Version, "commit": Commit, "time": time.Now().UTC()}
	if !a.Auth.Enabled || a.Auth.validSession(r) {
		response["services"] = ServiceStatus()
	}
	writeJSON(w, http.StatusOK, response)
}
func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	a.Metrics.Write(w)
}
func (a *App) services(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"services": ServiceSummaries(), "time": time.Now().UTC()})
}
func (a *App) securityAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"checks": a.SecurityChecks(), "time": time.Now().UTC()})
}
func (a *App) inspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUpload)
	file, header, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, "backup file is required or exceeds the upload limit", 400)
		return
	}
	defer file.Close()
	info, err := InspectCPMove(file, header)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	writeJSON(w, 200, info)
}
func (a *App) importBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid upload: "+err.Error(), 400)
		return
	}
	if r.FormValue("confirm") != "IMPORT" {
		http.Error(w, "type IMPORT to authorize restore", 400)
		return
	}
	if free, err := availableBytes(a.Config.ImportRoot); err == nil && free < a.Config.MinFreeBytes {
		http.Error(w, "insufficient free disk space for a restore", http.StatusInsufficientStorage)
		return
	}
	file, header, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, "backup file is required", 400)
		return
	}
	defer file.Close()
	user := safeUser(r.FormValue("username"))
	if user == "" {
		http.Error(w, "a valid account username is required", 400)
		return
	}
	temp, err := os.CreateTemp(a.Config.ImportRoot, "upload-*.tar.gz")
	if err != nil {
		http.Error(w, "could not stage upload", 500)
		return
	}
	tempPath := temp.Name()
	if _, err = io.Copy(temp, file); err != nil {
		temp.Close()
		http.Error(w, "could not stage upload", 500)
		return
	}
	if err = temp.Close(); err != nil {
		http.Error(w, "could not stage upload", 500)
		return
	}
	databaseRestore := r.FormValue("restore_databases") == "on"
	jobID := time.Now().UTC().Format("20060102-150405.000000000") + "-" + user
	a.Metrics.RestoreStarted()
	if err := a.Jobs.Submit(jobID, user, func() (ImportResult, error) {
		var restoreErr error
		defer func() { a.Metrics.RestoreFinished(restoreErr) }()
		defer os.Remove(tempPath)
		staged, openErr := os.Open(tempPath)
		if openErr != nil {
			restoreErr = openErr
			return ImportResult{}, openErr
		}
		defer staged.Close()
		result, restoreErr := RestoreCPMove(a.Config, staged, header, user, databaseRestore)
		if restoreErr != nil {
			_ = Audit(a.Config.AuditLog, "cpmove.restore.failed", user, restoreErr.Error())
		} else {
			_ = Audit(a.Config.AuditLog, "cpmove.restore.completed", user, result.StagedAt)
		}
		return result, restoreErr
	}); err != nil {
		_ = os.Remove(tempPath)
		a.Metrics.RestoreFinished(err)
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		} else {
			http.Error(w, "could not persist restore job", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status_url": filepath.Join("/api/jobs", jobID)})
}
func (a *App) jobStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	job, ok := a.Jobs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
func logging(next http.Handler, metrics *Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(wrapped, r)
		if metrics != nil {
			metrics.ObserveHTTP(wrapped.status, time.Since(started))
		}
		logJSON(r, wrapped.status, time.Since(started))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func logJSON(r *http.Request, status int, duration time.Duration) {
	if status == 0 {
		status = http.StatusOK
	}
	log.Printf(`{"level":"info","method":%q,"path":%q,"status":%d,"duration_ms":%.3f}`, r.Method, r.URL.Path, status, float64(duration.Microseconds())/1000)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func safeUser(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 32 || value == "" {
		return ""
	}
	for _, r := range value {
		if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return value
}
