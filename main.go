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
	"sync"
	"syscall"
	"time"
)

type App struct {
	Config                   Config
	View                     *template.Template
	Auth                     Auth
	Jobs                     *Jobs
	Metrics                  *Metrics
	Schedules                *backupSchedules
	Accounts                 *AccountStore
	RecoveryError            error
	Environments             *EnvironmentStore
	Redis                    *RedisAllocationStore
	Access                   *SiteAccessStore
	Workers                  *WorkerStore
	Composer                 *ComposerStore
	PHP                      *PHPProfileStore
	Tasks                    *TaskStore
	Deployments              *DeploymentStore
	Resources                *ResourceStore
	databaseDiagnosticsMu    sync.Mutex
	databaseDiagnosticsCache DatabaseDiagnostics
	gitActivationMu          sync.Mutex
	siteOperations           siteOperationLocks
	appLifecycleMu           sync.Mutex
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		_, _ = fmt.Fprintf(os.Stdout, "StePanel %s\ncommit: %s\nbuilt: %s\n", Version, Commit, BuildDate)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "convert-htaccess" {
		content, err := io.ReadAll(io.LimitReader(os.Stdin, maxHTAccessBytes+1))
		if err != nil || len(content) > maxHTAccessBytes {
			log.Fatal(".htaccess input exceeds the 256 KiB limit")
		}
		conversion, err := translateHTAccess(string(content))
		if err != nil {
			log.Fatal(err)
		}
		_, _ = fmt.Fprint(os.Stdout, conversion.CaddyDirectives)
		for _, warning := range conversion.Warnings {
			_, _ = fmt.Fprintln(os.Stderr, "warning:", warning)
		}
		if len(conversion.Warnings) > 0 {
			os.Exit(2)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "verify-backup" {
		manifest, err := VerifySiteBackup(os.Args[2], LoadConfig().BackupSigningKey)
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
	if len(os.Args) == 2 && os.Args[1] == "dr-check" {
		if err := runDRCheck(LoadConfig()); err != nil {
			log.Fatal(err)
		}
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
	if cfg.Production && os.Getenv("STEPANEL_ACCOUNT_STATE") == "" {
		cfg.AccountState = filepath.Join(filepath.Dir(cfg.SessionState), "accounts.json")
	}
	if cfg.Production && strings.TrimSpace(cfg.AccountKey) == "" {
		log.Fatal("production requires STEPANEL_ACCOUNT_KEY to encrypt customer TOTP secrets")
	}
	if err := ValidateConfig(cfg); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	if strings.TrimSpace(cfg.AccountState) == "" || strings.ContainsAny(cfg.AccountState, "\x00\r\n") || cfg.Production && !filepath.IsAbs(cfg.AccountState) {
		log.Fatal("STEPANEL_ACCOUNT_STATE must be a non-empty filesystem path and absolute in production")
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
	}{{cfg.ImportRoot, 0700}, {cfg.BackupRoot, 0700}, {filepath.Dir(cfg.JobState), 0750}, {filepath.Dir(cfg.SessionState), 0750}, {filepath.Dir(cfg.AccountState), 0750}, {cfg.RecoveryRoot, 0700}} {
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
	accounts, err := OpenAccountStore(cfg.AccountState, cfg.AccountKey)
	if err != nil {
		log.Fatalf("open persistent shared-hosting account state: %v", err)
	}
	auth.Accounts = accounts
	environments, err := OpenEnvironmentStore(cfg.EnvironmentState, cfg.EnvironmentKey)
	if err != nil {
		log.Fatalf("open site environment state: %v", err)
	}
	redisAllocations, err := OpenRedisAllocationStore(cfg.RedisState)
	if err != nil {
		log.Fatalf("open Redis allocation state: %v", err)
	}
	access, err := OpenSiteAccessStore(siteAccessStatePath(cfg))
	if err != nil {
		log.Fatalf("open site SSH access state: %v", err)
	}
	workers, err := OpenWorkerStore(filepath.Join(filepath.Dir(cfg.JobState), "workers.json"))
	if err != nil {
		log.Fatalf("open worker state: %v", err)
	}
	composer, err := OpenComposerStore(filepath.Join(filepath.Dir(cfg.JobState), "composer-operations.json"))
	if err != nil {
		log.Fatalf("open Composer state: %v", err)
	}
	phpProfiles, err := OpenPHPProfileStore(filepath.Join(filepath.Dir(cfg.JobState), "php-profiles.json"))
	if err != nil {
		log.Fatalf("open PHP profile state: %v", err)
	}
	tasks, err := OpenTaskStore(filepath.Join(filepath.Dir(cfg.JobState), "scheduled-tasks.json"))
	if err != nil {
		log.Fatalf("open scheduled task state: %v", err)
	}
	deployments, err := OpenDeploymentStore(filepath.Join(filepath.Dir(cfg.JobState), "deployments.json"))
	if err != nil {
		log.Fatalf("open deployment state: %v", err)
	}
	resources, err := OpenResourceStore(filepath.Join(filepath.Dir(cfg.JobState), "resource-profiles.json"))
	if err != nil {
		log.Fatalf("open resource profile state: %v", err)
	}
	if cfg.DBCtl != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		output, err := runBoundedCommand(ctx, helperCommandContext(ctx, cfg, cfg.DBCtl, "reconcile"))
		cancel()
		if err != nil {
			log.Fatalf("reconcile interrupted database operations: %v: %s", err, strings.TrimSpace(string(output)))
		}
	}
	var recoveryFailures []error
	databaseRecoveries, err := RecoverTransactionDatabases(cfg, cfg.RecoveryRoot)
	if err != nil {
		recoveryFailures = append(recoveryFailures, err)
		log.Printf("recover interrupted database transactions (continuing with isolated failures): %v", err)
	}
	for _, id := range databaseRecoveries {
		log.Printf("recovered databases for interrupted site transaction %s", id)
		_ = Audit(cfg.AuditLog, "restore.database-recovered", id, "managed databases removed after unclean shutdown")
	}
	recovered, err := RecoverSiteTransactions(cfg.RecoveryRoot)
	if err != nil {
		recoveryFailures = append(recoveryFailures, err)
		log.Printf("recover interrupted site transactions (continuing with isolated failures): %v", err)
	}
	for _, id := range recovered {
		txn, loadErr := loadSiteTransaction(filepath.Join(cfg.RecoveryRoot, id))
		if loadErr != nil {
			recoveryFailures = append(recoveryFailures, fmt.Errorf("load recovered site transaction %s: %w", id, loadErr))
			log.Printf("load recovered site transaction %s: %v", id, loadErr)
			continue
		}
		if sealErr := siteHelper(cfg, "seal", txn.Site); sealErr != nil {
			recoveryFailures = append(recoveryFailures, fmt.Errorf("seal recovered site transaction %s: %w", id, sealErr))
			log.Printf("seal recovered site transaction %s: %v", id, sealErr)
			continue
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
	schedules, err := openBackupSchedules(filepath.Join(filepath.Dir(cfg.JobState), "backup-schedules.json"))
	if err != nil {
		log.Fatalf("open backup schedules: %v", err)
	}
	app := &App{Config: cfg, View: view, Auth: auth, Jobs: jobs, Metrics: NewMetrics(), Schedules: schedules, Accounts: accounts, Environments: environments, Redis: redisAllocations, Access: access, Workers: workers, Composer: composer, PHP: phpProfiles, Tasks: tasks, Deployments: deployments, Resources: resources, RecoveryError: errors.Join(recoveryFailures...)}
	reconcileCtx, cancelReconcile := context.WithTimeout(context.Background(), helperCommandTimeout)
	if reconciled, failed := app.reconcileSiteAccess(reconcileCtx); len(failed) > 0 {
		log.Printf("SSH access reconciliation incomplete: reconciled=%d failed=%d", len(reconciled), len(failed))
	}
	if reconciled, failed := app.reconcileWorkers(reconcileCtx); len(failed) > 0 {
		log.Printf("worker reconciliation incomplete: reconciled=%d failed=%d", len(reconciled), len(failed))
	}
	if reconciled, failed := app.reconcilePHPProfiles(reconcileCtx); len(failed) > 0 {
		log.Printf("PHP profile reconciliation incomplete: reconciled=%d failed=%d", len(reconciled), len(failed))
	}
	if reconciled, failed := app.reconcilePythonApps(reconcileCtx); len(failed) > 0 {
		log.Printf("Python application reconciliation incomplete: reconciled=%d failed=%d", len(reconciled), len(failed))
	}
	if reconciled, failed := app.reconcileTasks(reconcileCtx); len(failed) > 0 {
		log.Printf("scheduled-task reconciliation incomplete: reconciled=%d failed=%d", len(reconciled), len(failed))
	}
	if reconciled, failed := app.reconcileEnvironments(reconcileCtx); len(failed) > 0 {
		log.Printf("environment reconciliation incomplete: reconciled=%d failed=%d", len(reconciled), len(failed))
	}
	cancelReconcile()
	if err := pruneAllGitReleases(cfg); err != nil {
		log.Printf("Git release retention during startup: %v", err)
	}
	if err := Audit(cfg.AuditLog, "service.started", "stepanel", "control plane initialized"); err != nil {
		log.Printf("initialize audit chain: %v", err)
	}
	if !app.Auth.Enabled {
		log.Println("warning: authentication is disabled; set STEPANEL_ADMIN_PASSWORD and STEPANEL_SESSION_SECRET")
	}
	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		scheduleTicker := time.NewTicker(time.Minute)
		cleanupTicker := time.NewTicker(15 * time.Minute)
		defer scheduleTicker.Stop()
		defer cleanupTicker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-scheduleTicker.C:
				app.runDueBackups()
			case <-cleanupTicker.C:
				app.Jobs.Cleanup(24 * time.Hour)
				if err := CleanupImportStages(app.Config.ImportRoot, time.Duration(app.Config.StageRetentionHours)*time.Hour); err != nil {
					log.Printf("import stage cleanup: %v", err)
				}
				if err := CleanupSiteTransactions(app.Config.RecoveryRoot, time.Duration(app.Config.StageRetentionHours)*time.Hour); err != nil {
					log.Printf("site recovery cleanup: %v", err)
				}
				app.gitActivationMu.Lock()
				if err := pruneAllGitReleases(app.Config); err != nil {
					log.Printf("Git release retention: %v", err)
				}
				app.gitActivationMu.Unlock()
			}
		}
	}()
	mux := http.NewServeMux()
	expensive := make(chan struct{}, 4)
	uploads := make(chan struct{}, max(1, cfg.MaxConcurrentJobs))
	mux.Handle("/livez", allowMethods(http.HandlerFunc(app.livez), http.MethodGet, http.MethodHead))
	mux.Handle("/readyz", allowMethods(http.HandlerFunc(app.readyz), http.MethodGet, http.MethodHead))
	mux.Handle("/static/", allowMethods(http.StripPrefix("/static/", http.FileServer(http.FS(staticAssets))), http.MethodGet, http.MethodHead))
	mux.Handle("/login", allowMethods(http.HandlerFunc(app.Auth.Login), http.MethodGet, http.MethodPost))
	mux.Handle("/logout", allowMethods(http.HandlerFunc(app.Auth.Logout), http.MethodPost))
	mux.Handle("/", allowMethods(app.Auth.Require(http.HandlerFunc(app.dashboard)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/health", allowMethods(http.HandlerFunc(app.health), http.MethodGet, http.MethodHead))
	mux.Handle("/api/services", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.services)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/database", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.database)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/database/diagnostics", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.databaseDiagnostics)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/database/sessions", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.databaseSessions)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/database/sessions/", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.databaseSessionTerminate)), http.MethodDelete))
	mux.Handle("/api/database/settings", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.databaseSettings)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/databases", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.databaseCollection)), http.MethodGet, http.MethodHead, http.MethodPost))
	mux.Handle("/api/databases/", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.databaseResource)), http.MethodGet, http.MethodHead, http.MethodPatch, http.MethodDelete))
	mux.Handle("/api/ftp", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.ftpStatus)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/security/audit", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.securityAudit)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/security/center", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.securityCenter)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/audit/events", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.auditEvents)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/doctor", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.doctor)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/cloud", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.cloudInventory)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/cloud/action", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.cloudAction)), http.MethodPost))
	mux.Handle("/api/cloud/dns", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.cloudDNS)), http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete))
	mux.Handle("/api/dns/capabilities", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.dnsCapabilities)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/cloud/loadbalancer", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.cloudLoadBalancer)), http.MethodPost))
	mux.Handle("/api/cloud/snapshots", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.cloudSnapshots)), http.MethodGet, http.MethodHead, http.MethodDelete))
	mux.Handle("/api/ssh", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.sshInventory)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/ssh/action", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.sshAction)), http.MethodPost))
	mux.Handle("/api/capabilities", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.capabilities)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/security/scan", allowMethods(app.Auth.RequireAdministrator(limitConcurrent(http.HandlerFunc(app.malwareScan), expensive)), http.MethodPost))
	mux.Handle("/api/certificates/issue", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.issueCertificate)), http.MethodPost))
	mux.Handle("/api/node/versions", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.nodeVersions)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/node/select", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.selectNode)), http.MethodPost))
	mux.Handle("/api/node/tooling", allowMethods(app.Auth.Require(http.HandlerFunc(app.nodeTooling)), http.MethodPost))
	mux.Handle("/api/proxy/deploy", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.deployProxy)), http.MethodPost))
	mux.Handle("/api/proxy", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.proxyList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/proxy/test", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.proxyTest)), http.MethodPost))
	mux.Handle("/api/proxy/", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.proxyManage)), http.MethodDelete))
	mux.Handle("/api/sites", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.siteList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/sites/overview", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteOverviewList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/sites/overview/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteOverviewResource)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/sites/environment/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteEnvironment)), http.MethodGet, http.MethodPut, http.MethodDelete))
	mux.Handle("/api/sites/redis/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteRedis)), http.MethodGet, http.MethodPut, http.MethodDelete))
	mux.Handle("/api/sites/access/", allowMethods(app.Auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			app.siteAccessKey(w, r)
			return
		}
		app.siteAccess(w, r)
	})), http.MethodGet, http.MethodPatch, http.MethodPost, http.MethodDelete))
	mux.Handle("/api/workers/", allowMethods(app.Auth.Require(http.HandlerFunc(app.workers)), http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete))
	mux.Handle("/api/tasks/", allowMethods(app.Auth.Require(http.HandlerFunc(app.tasks)), http.MethodGet, http.MethodPut, http.MethodDelete))
	mux.Handle("/api/python/deploy", allowMethods(app.Auth.Require(http.HandlerFunc(app.pythonDeploy)), http.MethodPost))
	mux.Handle("/api/python/", allowMethods(app.Auth.Require(http.HandlerFunc(app.pythonAction)), http.MethodPost))
	mux.Handle("/api/composer/", allowMethods(app.Auth.Require(http.HandlerFunc(app.composer)), http.MethodGet, http.MethodHead, http.MethodPost))
	mux.Handle("/api/sites/php/", allowMethods(app.Auth.Require(http.HandlerFunc(app.phpRuntime)), http.MethodGet, http.MethodHead, http.MethodPut))
	mux.Handle("/api/sites/resources/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteResources)), http.MethodGet, http.MethodHead, http.MethodPut))
	mux.Handle("/api/sites/usage/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteUsage)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/reconcile/resources", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.reconcileResources)), http.MethodPost))
	mux.Handle("/api/reconcile/tasks", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.reconcileTasksHTTP)), http.MethodPost))
	mux.Handle("/api/staging", allowMethods(app.Auth.Require(http.HandlerFunc(app.stagingCreate)), http.MethodPost))
	mux.Handle("/api/runner/build", allowMethods(app.Auth.Require(http.HandlerFunc(app.runnerBuild)), http.MethodPost))
	mux.Handle("/api/deployments", allowMethods(app.Auth.Require(http.HandlerFunc(app.deployments)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/deployments/run", allowMethods(app.Auth.Require(http.HandlerFunc(app.releasePipeline)), http.MethodPost))
	mux.Handle("/api/sites/logs/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteLogs)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/sites/deploy", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteDeploy)), http.MethodPost))
	mux.Handle("/api/sites/", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.siteManage)), http.MethodDelete))
	mux.Handle("/api/backups", app.Auth.Require(http.HandlerFunc(app.backups)))
	mux.Handle("/api/backups/restore-to-staging", allowMethods(app.Auth.Require(http.HandlerFunc(app.backupRestoreToStaging)), http.MethodPost))
	mux.Handle("/api/backups/restore-offsite-to-staging", allowMethods(app.Auth.Require(http.HandlerFunc(app.backupRestoreOffsiteToStaging)), http.MethodPost))
	mux.Handle("/api/backups/restore-files", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.backupRestoreFilesHTTP)), http.MethodPost))
	mux.Handle("/api/backups/restore-database", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.backupRestoreDatabaseHTTP)), http.MethodPost))
	mux.Handle("/api/backups/restore-offsite-files", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.backupRestoreOffsiteFilesHTTP)), http.MethodPost))
	mux.Handle("/api/backups/restore-offsite-database", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.backupRestoreOffsiteDatabaseHTTP)), http.MethodPost))
	mux.Handle("/api/backups/verify", allowMethods(app.Auth.Require(http.HandlerFunc(app.backupVerify)), http.MethodPost))
	mux.Handle("/api/backup-schedules", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.backupSchedules)), http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete))
	mux.Handle("/api/apps", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.appList)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/apps/deploy", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.appDeploy)), http.MethodPost))
	mux.Handle("/api/sites/git-deploy", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.gitDeploy)), http.MethodPost))
	mux.Handle("/api/sites/git-key/", allowMethods(app.Auth.Require(http.HandlerFunc(app.siteGitKey)), http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete))
	mux.Handle("/api/sites/git-webhook", allowMethods(http.HandlerFunc(app.gitWebhook), http.MethodPost))
	mux.Handle("/api/sites/git-rollback", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.gitRollback)), http.MethodPost))
	mux.Handle("/api/caddy/htaccess", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.htaccessMigration)), http.MethodPost))
	mux.Handle("/api/apps/", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.appAction)), http.MethodPost))
	mux.Handle("/api/cpmove/inspect", allowMethods(app.Auth.RequireAdministrator(limitConcurrent(http.HandlerFunc(app.inspect), expensive)), http.MethodPost))
	mux.Handle("/api/cpmove/import", allowMethods(app.Auth.RequireAdministrator(limitConcurrent(http.HandlerFunc(app.importBackup), uploads)), http.MethodPost))
	mux.Handle("/api/wpress/preflight", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.wpressPreflight)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/wpress/import", allowMethods(app.Auth.RequireAdministrator(limitConcurrent(http.HandlerFunc(app.wpressImport), uploads)), http.MethodPost))
	mux.Handle("/api/wordpress/status/", allowMethods(app.Auth.Require(http.HandlerFunc(app.wordpressStatus)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/wordpress/", allowMethods(app.Auth.Require(http.HandlerFunc(app.wordpressAction)), http.MethodPost))
	mux.Handle("/api/accounts", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.accounts)), http.MethodGet, http.MethodHead, http.MethodPost))
	mux.Handle("/api/accounts/", allowMethods(app.Auth.RequireAdministrator(http.HandlerFunc(app.accounts)), http.MethodPatch, http.MethodDelete, http.MethodPost))
	mux.Handle("/api/account/password", allowMethods(app.Auth.Require(http.HandlerFunc(app.customerPassword)), http.MethodPost))
	mux.Handle("/api/account/mfa", allowMethods(app.Auth.Require(http.HandlerFunc(app.customerMFA)), http.MethodPost))
	mux.Handle("/api/jobs/", allowMethods(app.Auth.Require(http.HandlerFunc(app.jobStatus)), http.MethodGet, http.MethodHead))
	mux.Handle("/api/jobs", allowMethods(app.Auth.Require(http.HandlerFunc(app.jobList)), http.MethodGet, http.MethodHead))
	metricsHandler := http.Handler(http.HandlerFunc(app.metrics))
	if os.Getenv("STEPANEL_METRICS_PUBLIC") != "1" {
		metricsHandler = app.Auth.RequireAdministrator(metricsHandler)
	}
	mux.Handle("/metrics", allowMethods(metricsHandler, http.MethodGet, http.MethodHead))
	server := &http.Server{Addr: cfg.Listen, Handler: logging(normalizeAPIErrors(mux), app.Metrics, cfg.Production), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, WriteTimeout: 30 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		log.Printf("StePanel listening on %s", cfg.Listen)
		var err error
		if cfg.TLSCertFile != "" {
			err = server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-runCtx.Done()
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
	isAdministrator := a.Auth.IsAdministrator(r)
	servers := ServiceSummaries(a.Config)
	jobs := a.Jobs.List(8)
	var account HostingAccount
	accountSiteCount := 0
	if !isAdministrator {
		servers = nil
		jobs = filterAccountJobs(jobs, a.Accounts, a.Auth.UsernameForRequest(r))
		if a.Accounts != nil {
			account, _ = a.Accounts.Get(a.Auth.UsernameForRequest(r))
			accountSiteCount = len(account.Sites)
		}
	}
	healthy, alerts := 0, 0
	for _, server := range servers {
		switch server.Status {
		case "active", "enabled", "installed":
			healthy++
		default:
			alerts++
		}
	}
	security := []SecurityCheck{}
	if isAdministrator {
		security = a.SecurityChecks()
	}
	if err := a.View.Execute(w, map[string]any{"Title": "StePanel", "Config": a.Config, "CSRF": csrf, "AuthEnabled": a.Auth.Enabled, "Username": a.Auth.UsernameForRequest(r), "Now": time.Now(), "Servers": servers, "Healthy": healthy, "Alerts": alerts, "Security": security, "Jobs": jobs, "Capabilities": a.Capabilities(), "Database": a.DatabaseAdmin(), "IsAdministrator": isAdministrator, "Account": account, "AccountSiteCount": accountSiteCount}); err != nil {
		log.Printf("dashboard render failed: %v", err)
	}
}

func filterAccountJobs(jobs []Job, accounts *AccountStore, username string) []Job {
	if accounts == nil {
		return nil
	}
	filtered := make([]Job, 0, len(jobs))
	for _, job := range jobs {
		if accounts.OwnsSite(username, job.User) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{"ok": true, "version": Version, "commit": Commit, "time": time.Now().UTC()}
	if !a.Auth.Enabled || a.Auth.IsAdministrator(r) {
		response["services"] = ServiceStatus()
	}
	writeJSON(w, http.StatusOK, response)
}
func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	a.Metrics.Write(w)
	writeDatabaseMetrics(w, a.cachedDatabaseDiagnostics(15*time.Second))
	if a.Schedules != nil {
		writeBackupScheduleMetrics(w, a.Schedules.list())
	}
	writeGitReleaseMetrics(w, a.Config.WebRoot)
}
func (a *App) services(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"services": ServiceSummaries(a.Config), "time": time.Now().UTC()})
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
		"certificates":       a.Config.WebServer == "apache" && commandAvailable(a.Config.Certbot),
		"mysql_restores":     mysqlCompatible(a.Config),
		"database_lifecycle": a.DatabaseAdmin().LifecycleReady,
		"database_pitr":      false,
		"database_failover":  false,
		"node_apps":          hasNode && commandAvailable(a.Config.AppCtl) && commandAvailable(a.Config.ProxyCtl),
		"wpress":             allReady(WPressPreflight(a.Config)),
		"htaccess_import":    a.Config.WebServer == "caddy" && commandAvailable(a.Config.VHostCtl),
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
	if a.Config.MaxUpload > 0 && r.ContentLength > a.Config.MaxUpload {
		http.Error(w, "upload exceeds the configured size limit", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUpload)
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
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
	if a.Config.MaxUpload > 0 && r.ContentLength > a.Config.MaxUpload {
		http.Error(w, "upload exceeds the configured size limit", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUpload)
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	if err := restoreCapacity(a.Config); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
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
	databaseRestore := r.FormValue("restore_databases") == "on"
	if databaseRestore && !mysqlCompatible(a.Config) {
		http.Error(w, "cPanel SQL restores require MySQL or MariaDB; PostgreSQL dump conversion is not supported", http.StatusUnprocessableEntity)
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
	operationKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if operationKey != "" && !validJobOperationKey(operationKey) {
		http.Error(w, "invalid Idempotency-Key", http.StatusUnprocessableEntity)
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
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		http.Error(w, "could not durably stage upload", 500)
		return
	}
	if err = temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		http.Error(w, "could not stage upload", 500)
		return
	}
	jobID, err := newJobID("cpmove")
	if err != nil {
		_ = os.Remove(tempPath)
		http.Error(w, "could not create restore job", http.StatusInternalServerError)
		return
	}
	queuedID, existing, err := a.Jobs.SubmitIdempotent(jobID, user, operationKey, func() (ImportResult, error) {
		a.Metrics.RestoreStarted()
		releaseUnlock := a.siteOperations.acquire(user)
		defer releaseUnlock()
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
				log.Printf("cpmove restore completed but audit persistence is unavailable: %v", auditErr)
			}
		}
		return result, restoreErr
	})
	if err != nil {
		_ = os.Remove(tempPath)
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		} else {
			http.Error(w, "could not persist restore job", http.StatusInternalServerError)
		}
		return
	}
	if existing {
		_ = os.Remove(tempPath)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": queuedID, "status_url": filepath.Join("/api/jobs", queuedID)})
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
	if !a.Auth.IsAdministrator(r) && !a.canAccessSite(r, job.User) {
		http.Error(w, "job is not assigned to this account", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
func (a *App) jobList(w http.ResponseWriter, r *http.Request) {
	jobs := a.Jobs.List(100)
	if !a.Auth.IsAdministrator(r) {
		jobs = filterAccountJobs(jobs, a.Accounts, a.Auth.UsernameForRequest(r))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "time": time.Now().UTC()})
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
