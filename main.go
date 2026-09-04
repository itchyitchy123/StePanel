package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
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
	if len(os.Args) == 3 && os.Args[1] == "verify-backup" {
		manifest, err := VerifySiteBackup(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s  %s\n", manifest.ArchiveSHA256, filepath.Join(os.Args[2], manifest.Archive))
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "verify-audit" {
		if err := VerifyAuditLog(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		_, _ = fmt.Fprintf(os.Stdout, "audit chain verified: %s\n", os.Args[2])
		return
	}
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
	if err := ValidateConfig(cfg); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	auth, err := NewAuth(cfg.Production)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Production && !auth.Enabled {
		log.Fatal("authentication must be configured in production")
	}
	auth.AuditLog = cfg.AuditLog
	for _, directory := range []struct {
		path string
		mode os.FileMode
	}{{cfg.ImportRoot, 0700}, {cfg.BackupRoot, 0700}, {filepath.Dir(cfg.JobState), 0750}, {filepath.Dir(cfg.SessionState), 0750}, {cfg.RecoveryRoot, 0700}} {
		if err := os.MkdirAll(directory.path, directory.mode); err != nil {
			log.Fatalf("initialize managed directory %s: %v", directory.path, err)
		}
	}
	processLock, err := acquireProcessLock(cfg.JobState + ".lock")
	if err != nil {
		log.Fatalf("acquire process lock: %v", err)
	}
	defer processLock.Close()
	if err := auth.ConfigureSessionStore(cfg.SessionState); err != nil {
		log.Fatalf("open persistent session state: %v", err)
	}
	if cfg.DBCtl != "" {
		if output, err := helperCommand(cfg, cfg.DBCtl, "reconcile").CombinedOutput(); err != nil {
			log.Fatalf("reconcile interrupted database operations: %v: %s", err, strings.TrimSpace(string(output)))
		}
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
	if err := CleanupImportStages(cfg.ImportRoot, time.Duration(cfg.StageRetentionHours)*time.Hour); err != nil {
		log.Printf("import stage cleanup during startup: %v", err)
	}
	if err := CleanupSiteTransactions(cfg.RecoveryRoot, time.Duration(cfg.StageRetentionHours)*time.Hour); err != nil {
		log.Printf("site recovery cleanup during startup: %v", err)
	}
	viewData, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		log.Fatalf("load embedded dashboard: %v", err)
	}
	view := template.Must(template.New("index.html").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(string(viewData)))
	staticAssets, err := fs.Sub(webAssets, "web/static")
	if err != nil {
		log.Fatalf("load embedded static assets: %v", err)
	}
	jobs, err := OpenJobs(cfg.JobState, cfg.MaxConcurrentJobs)
	if err != nil {
		log.Fatalf("open persistent job state: %v", err)
	}
	app := &App{Config: cfg, View: view, Auth: auth, Jobs: jobs, Metrics: NewMetrics()}
	if err := Audit(cfg.AuditLog, "service.started", "stepanel", "control plane initialized"); err != nil {
		log.Printf("initialize audit chain: %v", err)
	}
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
	expensive := make(chan struct{}, 4)
	mux.Handle("/livez", allowMethods(http.HandlerFunc(app.livez), http.MethodGet, http.MethodHead))
	mux.Handle("/readyz", allowMethods(http.HandlerFunc(app.readyz), http.MethodGet, http.MethodHead))
	mux.Handle("/static/", allowMethods(http.StripPrefix("/static/", http.FileServer(http.FS(staticAssets))), http.MethodGet, http.MethodHead))
	mux.Handle("/login", allowMethods(http.HandlerFunc(app.Auth.Login), http.MethodGet, http.MethodPost))
	mux.Handle("/logout", allowMethods(http.HandlerFunc(app.Auth.Logout), http.MethodPost))
	mux.Handle("/", allowMethods(app.Auth.Require(http.HandlerFunc(app.dashboard)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/health", allowMethods(http.HandlerFunc(app.health), http.MethodGet, http.MethodHead))
	mux.Handle("/api/services", allowMethods(app.Auth.Require(http.HandlerFunc(app.services)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/ftp", allowMethods(app.Auth.Require(http.HandlerFunc(app.ftpStatus)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/security/audit", allowMethods(app.Auth.Require(http.HandlerFunc(app.securityAudit)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/doctor", allowMethods(app.Auth.Require(http.HandlerFunc(app.doctor)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/capabilities", allowMethods(app.Auth.Require(http.HandlerFunc(app.capabilities)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/security/scan", allowMethods(app.Auth.Require(limitConcurrent(http.HandlerFunc(app.malwareScan), expensive)), http.MethodPost))
	mux.Handle("/api/certificates/issue", allowMethods(app.Auth.Require(http.HandlerFunc(app.issueCertificate)), http.MethodPost))
	mux.Handle("/api/node/versions", allowMethods(app.Auth.Require(http.HandlerFunc(app.nodeVersions)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/node/select", allowMethods(app.Auth.Require(http.HandlerFunc(app.selectNode)), http.MethodPost))
	mux.Handle("/api/proxy/deploy", allowMethods(app.Auth.Require(http.HandlerFunc(app.deployProxy)), http.MethodPost))
	mux.Handle("/api/proxy", allowMethods(app.Auth.Require(http.HandlerFunc(app.proxyList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/proxy/test", allowMethods(app.Auth.Require(http.HandlerFunc(app.proxyTest)), http.MethodPost))
	mux.Handle("/api/proxy/", allowMethods(app.Auth.Require(http.HandlerFunc(app.proxyManage)), http.MethodDelete))
	mux.Handle("/api/sites", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/sites/deploy", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteDeploy)), http.MethodPost))
	mux.Handle("/api/sites/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteManage)), http.MethodDelete))
	mux.Handle("/api/backups", app.Auth.Require(http.HandlerFunc(app.backups)))
	mux.Handle("/api/apps", allowMethods(app.Auth.Require(http.HandlerFunc(app.appList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/apps/deploy", allowMethods(app.Auth.Require(http.HandlerFunc(app.appDeploy)), http.MethodPost))
	mux.Handle("/api/apps/", allowMethods(app.Auth.Require(http.HandlerFunc(app.appAction)), http.MethodPost))
	mux.Handle("/api/cpmove/inspect", allowMethods(app.Auth.Require(limitConcurrent(http.HandlerFunc(app.inspect), expensive)), http.MethodPost))
	mux.Handle("/api/cpmove/import", allowMethods(app.Auth.Require(http.HandlerFunc(app.importBackup)), http.MethodPost))
	mux.Handle("/api/wpress/preflight", allowMethods(app.Auth.Require(http.HandlerFunc(app.wpressPreflight)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/wpress/import", allowMethods(app.Auth.Require(http.HandlerFunc(app.wpressImport)), http.MethodPost))
	mux.Handle("/api/jobs/", allowMethods(app.Auth.Require(http.HandlerFunc(app.jobStatus)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/jobs", allowMethods(app.Auth.Require(http.HandlerFunc(app.jobList)), http.MethodGet, http.MethodHead))
	metricsHandler := http.Handler(http.HandlerFunc(app.metrics))
	if os.Getenv("STEPANEL_METRICS_PUBLIC") != "1" {
		metricsHandler = app.Auth.Require(metricsHandler)
	}
	mux.Handle("/metrics", allowMethods(metricsHandler, http.MethodGet, http.MethodHead))
	server := &http.Server{Addr: cfg.Listen, Handler: logging(normalizeAPIErrors(mux), app.Metrics, cfg.Production), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, WriteTimeout: 30 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
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
	servers := ServiceSummaries()
	healthy, alerts := 0, 0
	for _, server := range servers {
		switch server.Status {
		case "active", "enabled", "installed":
			healthy++
		default:
			alerts++
		}
	}
	if err := a.View.Execute(w, map[string]any{"Title": "StePanel", "Config": a.Config, "CSRF": csrf, "AuthEnabled": a.Auth.Enabled, "Username": a.Auth.Username, "Now": time.Now(), "Servers": servers, "Healthy": healthy, "Alerts": alerts, "Security": a.SecurityChecks(), "Jobs": a.Jobs.List(8), "Capabilities": a.Capabilities()}); err != nil {
		log.Printf("dashboard render failed: %v", err)
	}
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
func (a *App) Capabilities() map[string]bool {
	nodeVersions, _ := os.ReadDir(filepath.Join(a.Config.NVMDir, "versions", "node"))
	hasNode := false
	for _, entry := range nodeVersions {
		if entry.IsDir() && nodeVersionPattern.MatchString(entry.Name()) {
			hasNode = true
			break
		}
	}
	return map[string]bool{
		"certificates": commandAvailable(a.Config.Certbot),
		"node_apps":    hasNode && commandAvailable(a.Config.AppCtl) && commandAvailable(a.Config.ProxyCtl),
		"wpress":       allReady(WPressPreflight(a.Config)),
	}
}
func (a *App) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": a.Capabilities(), "time": time.Now().UTC()})
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
	defer cleanupMultipartForm(r)
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
	err := r.ParseMultipartForm(32 << 20)
	defer cleanupMultipartForm(r)
	if err != nil {
		http.Error(w, "invalid upload: "+err.Error(), 400)
		return
	}
	if r.FormValue("confirm") != "IMPORT" {
		http.Error(w, "type IMPORT to authorize restore", 400)
		return
	}
	if err := restoreCapacity(a.Config); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
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
		_ = os.Remove(tempPath)
		http.Error(w, "could not stage upload", 500)
		return
	}
	if err = temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		http.Error(w, "could not stage upload", 500)
		return
	}
	databaseRestore := r.FormValue("restore_databases") == "on"
	jobID, err := newJobID("cpmove")
	if err != nil {
		_ = os.Remove(tempPath)
		http.Error(w, "could not create restore job", http.StatusInternalServerError)
		return
	}
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
			if auditErr := AuditAs(a.Config.AuditLog, a.Auth.Username, "cpmove.restore.failed", user, restoreErr.Error()); auditErr != nil {
				restoreErr = fmt.Errorf("%w; audit persistence failed: %v", restoreErr, auditErr)
			}
		} else {
			if auditErr := AuditAs(a.Config.AuditLog, a.Auth.Username, "cpmove.restore.completed", user, result.StagedAt); auditErr != nil {
				restoreErr = auditErr
			}
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
func (a *App) jobList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": a.Jobs.List(100), "time": time.Now().UTC()})
}
func logging(next http.Handler, metrics *Metrics, production bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID, err := randomSecret()
		if err != nil {
			requestID = fmt.Sprintf("fallback-%d", started.UnixNano())
		} else if len(requestID) > 20 {
			requestID = requestID[:20]
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, requestID))
		wrapped := &statusWriter{ResponseWriter: w}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		if production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(wrapped, r)
		if wrapped.status == 0 {
			wrapped.status = http.StatusOK
		}
		if metrics != nil {
			metrics.ObserveHTTP(wrapped.status, time.Since(started))
		}
		logJSON(r, wrapped.status, time.Since(started))
	})
}

func allowMethods(next http.Handler, methods ...string) http.Handler {
	allowed := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		allowed[method] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := allowed[r.Method]; !ok {
			w.Header().Set("Allow", strings.Join(methods, ", "))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func limitConcurrent(next http.Handler, slots chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "server is busy; retry shortly", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func cleanupMultipartForm(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
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
	requestID, _ := r.Context().Value(requestIDContextKey{}).(string)
	log.Printf(`{"level":"info","request_id":%q,"method":%q,"path":%q,"status":%d,"duration_ms":%.3f}`, requestID, r.Method, r.URL.Path, status, float64(duration.Microseconds())/1000)
}

type requestIDContextKey struct{}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *bufferedResponse) Header() http.Header { return w.header }
func (w *bufferedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *bufferedResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

// normalizeAPIErrors preserves existing handlers while giving clients one
// predictable JSON error envelope. API responses are small and never streamed.
func normalizeAPIErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		captured := &bufferedResponse{header: make(http.Header)}
		next.ServeHTTP(captured, r)
		status := captured.status
		if status == 0 {
			status = http.StatusOK
		}
		for name, values := range captured.header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		if status >= 400 && strings.HasPrefix(captured.header.Get("Content-Type"), "text/plain") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": strings.TrimSpace(captured.body.String())})
			return
		}
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(captured.body.Bytes())
		}
	})
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
