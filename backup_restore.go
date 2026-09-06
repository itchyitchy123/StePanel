package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RestoreToStagingRequest struct {
	Backup string `json:"backup"`
	Site   string `json:"site"`
	Domain string `json:"domain"`
}

type BackupRestoreResult struct {
	Site              string    `json:"site"`
	Backup            string    `json:"backup"`
	Mode              string    `json:"mode"`
	Database          string    `json:"database,omitempty"`
	FilesRestored     bool      `json:"files_restored"`
	DatabaseRestored  bool      `json:"database_restored"`
	DatabasePreserved bool      `json:"database_preserved"`
	SafetyBackup      string    `json:"safety_backup,omitempty"`
	Consistency       string    `json:"consistency"`
	SchemaRollback    string    `json:"schema_rollback"`
	CompletedAt       time.Time `json:"completed_at"`
}

// backupVerify performs the same archive and manifest checks used before a
// restore, without extracting or changing host state.
func (a *App) backupVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	var input struct {
		Backup string `json:"backup"`
		Site   string `json:"site"`
	}
	if err := decodeJSON(w, r, 4096, &input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	input.Site = safeUser(input.Site)
	input.Backup = filepath.Base(strings.TrimSpace(input.Backup))
	if input.Site == "" || input.Backup == "" || input.Backup == "." || !a.canAccessSite(r, input.Site) {
		http.Error(w, "invalid backup", http.StatusUnprocessableEntity)
		return
	}
	path := filepath.Join(a.Config.BackupRoot, input.Backup)
	if filepath.Dir(path) != filepath.Clean(a.Config.BackupRoot) {
		http.Error(w, "invalid backup", http.StatusUnprocessableEntity)
		return
	}
	manifest, err := VerifySiteBackup(path, a.Config.BackupSigningKey)
	if err != nil {
		http.Error(w, "backup verification failed", http.StatusUnprocessableEntity)
		return
	}
	if manifest.Site != input.Site {
		http.Error(w, "backup does not belong to site", http.StatusForbidden)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "backup.verify", input.Site, input.Backup)
	writeJSON(w, http.StatusOK, map[string]any{"verified": true, "backup": input.Backup, "site": manifest.Site, "consistency": manifest.Consistency, "archive_verified": manifest.ArchiveVerified, "database_dump_verified": manifest.DatabaseDumpVerified, "application_quiesced": manifest.ApplicationQuiesced, "filesystem_snapshot": manifest.FilesystemSnapshot, "manifest_signed": manifest.SignatureAlgorithm != ""})
}

func (a *App) backupRestoreToStaging(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var input RestoreToStagingRequest
	if e := decodeJSON(w, r, 4096, &input); e != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	input.Site = safeUser(input.Site)
	input.Backup = filepath.Base(strings.TrimSpace(input.Backup))
	input.Domain = strings.ToLower(strings.TrimSpace(input.Domain))
	if input.Site == "" || input.Backup == "." || input.Backup == "" || !domainPattern.MatchString(input.Domain) || !a.canAccessSite(r, input.Site) {
		http.Error(w, "invalid restore destination", 422)
		return
	}
	backup := filepath.Join(a.Config.BackupRoot, input.Backup)
	if filepath.Dir(backup) != filepath.Clean(a.Config.BackupRoot) {
		http.Error(w, "invalid backup", 422)
		return
	}
	manifest, e := VerifySiteBackup(backup, a.Config.BackupSigningKey)
	if e != nil {
		http.Error(w, "backup verification failed", 422)
		return
	}
	dest := filepath.Join(a.Config.WebRoot, "sites", input.Site, "public")
	if e = ensureInside(a.Config.WebRoot, dest); e != nil {
		http.Error(w, "invalid destination", 422)
		return
	}
	if _, e = os.Stat(dest); e == nil {
		http.Error(w, "staging destination already exists", 409)
		return
	}
	stage, e := os.MkdirTemp(a.Config.ImportRoot, "backup-restore-")
	if e != nil {
		http.Error(w, "could not prepare restore staging", 500)
		return
	}
	defer os.RemoveAll(stage)
	if e = extractArchive(filepath.Join(backup, manifest.Archive), stage); e != nil {
		http.Error(w, "could not extract verified backup", 502)
		return
	}
	source := filepath.Join(stage, "site", "public")
	if e = ensureInside(stage, source); e != nil {
		http.Error(w, "invalid backup layout", 422)
		return
	}
	if info, statErr := os.Stat(source); statErr != nil || !info.IsDir() {
		http.Error(w, "backup has no site files", 422)
		return
	}
	if e = siteHelper(a.Config, "prepare", input.Site); e != nil {
		http.Error(w, "could not prepare isolated staging site", 502)
		return
	}
	if e = writeAtomic(filepath.Join(a.Config.WebRoot, "sites", input.Site, ".stepanel-staging-noindex"), []byte("managed restore staging noindex\n"), 0600); e != nil {
		http.Error(w, "could not apply restore indexing protection", 503)
		return
	}
	txn, e := BeginSiteTransaction(a.Config.RecoveryRoot, dest, "backup.restore-to-staging", input.Site)
	if e != nil {
		http.Error(w, "could not journal restore", 503)
		return
	}
	ok := false
	defer func() {
		if !ok {
			_ = txn.Rollback()
		}
	}()
	if e = copyTree(source, dest); e != nil {
		http.Error(w, "could not restore site files", 502)
		return
	}
	if e = siteHelper(a.Config, "seal", input.Site); e != nil {
		http.Error(w, "could not seal restored site", 502)
		return
	}
	if e = runHelperCommand(r.Context(), a.Config, a.Config.VHostCtl, "apply", input.Site, input.Domain); e != nil {
		http.Error(w, "could not activate restored staging route", 502)
		return
	}
	if e = txn.Commit(); e != nil {
		http.Error(w, "could not commit restore", 503)
		return
	}
	ok = true
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "backup.restore-to-staging", input.Site, input.Backup)
	writeJSON(w, 202, map[string]any{"site": input.Site, "domain": input.Domain, "backup": input.Backup, "source_site": manifest.Site, "files_restored": true, "databases_restored": false, "restore_mode": "staging", "consistency": manifest.Consistency, "created_at": time.Now().UTC()})
}

// backupRestoreFiles replaces only the managed site files. It deliberately
// leaves databases untouched and uses the normal site recovery journal so an
// interrupted extraction can be resumed or rolled back by the operator.
func backupRestoreFiles(cfg Config, backupName, site string) (BackupRestoreResult, error) {
	backup := filepath.Join(cfg.BackupRoot, filepath.Base(backupName))
	if filepath.Dir(backup) != filepath.Clean(cfg.BackupRoot) {
		return BackupRestoreResult{}, errors.New("invalid backup path")
	}
	manifest, err := VerifySiteBackup(backup, cfg.BackupSigningKey)
	if err != nil {
		return BackupRestoreResult{}, fmt.Errorf("verify backup: %w", err)
	}
	if err := os.MkdirAll(cfg.ImportRoot, 0700); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("create restore staging root: %w", err)
	}
	stage, err := os.MkdirTemp(cfg.ImportRoot, "backup-files-restore-")
	if err != nil {
		return BackupRestoreResult{}, err
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(filepath.Join(backup, manifest.Archive), stage); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("extract verified backup: %w", err)
	}
	source := filepath.Join(stage, "site", "public")
	if err := ensureInside(stage, source); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("invalid backup layout: %w", err)
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return BackupRestoreResult{}, errors.New("backup has no site files")
	}
	dest := filepath.Join(cfg.WebRoot, "sites", site, "public")
	if err := ensureInside(cfg.WebRoot, dest); err != nil {
		return BackupRestoreResult{}, err
	}
	txn, err := BeginSiteTransaction(cfg.RecoveryRoot, dest, "backup.restore-files", site)
	if err != nil {
		return BackupRestoreResult{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = txn.Rollback()
		}
	}()
	if err := siteHelper(cfg, "prepare", site); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("prepare site: %w", err)
	}
	if err := copyTree(source, dest); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("restore site files: %w", err)
	}
	if err := siteHelper(cfg, "seal", site); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("seal site: %w", err)
	}
	if err := txn.Commit(); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("commit restore journal: %w", err)
	}
	ok = true
	return BackupRestoreResult{Site: site, Backup: filepath.Base(backup), Mode: "files-only", FilesRestored: true, DatabasePreserved: true, Consistency: manifest.Consistency, CompletedAt: time.Now().UTC()}, nil
}

func (a *App) backupRestoreFilesHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) || !a.Auth.IsAdministrator(r) {
		http.Error(w, "administrator CSRF request required", http.StatusForbidden)
		return
	}
	var input struct {
		Backup  string `json:"backup"`
		Site    string `json:"site"`
		Confirm string `json:"confirm"`
	}
	if err := decodeJSON(w, r, 4096, &input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	input.Site = safeUser(input.Site)
	input.Backup = filepath.Base(strings.TrimSpace(input.Backup))
	if input.Site == "" || input.Backup == "" || input.Backup == "." || input.Confirm != "RESTORE_FILES" {
		http.Error(w, "site, backup, and confirm=RESTORE_FILES are required", http.StatusUnprocessableEntity)
		return
	}
	if a.Jobs == nil {
		http.Error(w, "job system unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err := VerifySiteBackup(filepath.Join(a.Config.BackupRoot, input.Backup), a.Config.BackupSigningKey); err != nil {
		http.Error(w, "backup verification failed", http.StatusUnprocessableEntity)
		return
	}
	jobID, err := newJobID("backup-restore")
	if err != nil {
		http.Error(w, "could not create restore job", http.StatusInternalServerError)
		return
	}
	if err := a.Jobs.SubmitBackupRestore(jobID, input.Site, func() (BackupRestoreResult, error) {
		result, restoreErr := backupRestoreFiles(a.Config, input.Backup, input.Site)
		if restoreErr != nil {
			_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "backup.restore-files.failed", input.Site, restoreErr.Error())
			return result, restoreErr
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "backup.restore-files.completed", input.Site, input.Backup)
		return result, nil
	}); err != nil {
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		} else {
			http.Error(w, "could not persist restore job", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status_url": "/api/jobs/" + jobID, "mode": "files-only"})
}

func backupContainsDatabase(manifest BackupManifest, database string) bool {
	for _, name := range manifest.Databases {
		if name == database {
			return true
		}
	}
	return false
}

func restoreManagedDatabase(cfg Config, backupName, site, database string) (BackupRestoreResult, error) {
	backup := filepath.Join(cfg.BackupRoot, filepath.Base(backupName))
	if filepath.Dir(backup) != filepath.Clean(cfg.BackupRoot) {
		return BackupRestoreResult{}, errors.New("invalid backup path")
	}
	manifest, err := VerifySiteBackup(backup, cfg.BackupSigningKey)
	if err != nil {
		return BackupRestoreResult{}, fmt.Errorf("verify backup: %w", err)
	}
	if !backupContainsDatabase(manifest, database) {
		return BackupRestoreResult{}, errors.New("backup does not contain the selected database")
	}
	if cfg.DBCtl == "" {
		return BackupRestoreResult{}, errors.New("managed database helper is not configured")
	}
	if err := os.MkdirAll(cfg.ImportRoot, 0700); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("create restore staging root: %w", err)
	}
	stage, err := os.MkdirTemp(cfg.ImportRoot, "backup-database-restore-")
	if err != nil {
		return BackupRestoreResult{}, err
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(filepath.Join(backup, manifest.Archive), stage); err != nil {
		return BackupRestoreResult{}, fmt.Errorf("extract verified backup: %w", err)
	}
	dump := filepath.Join(stage, "databases", database+".sql")
	if err := ensureInside(stage, dump); err != nil {
		return BackupRestoreResult{}, err
	}
	info, err := os.Stat(dump)
	if err != nil || !info.Mode().IsRegular() {
		return BackupRestoreResult{}, errors.New("selected database dump is unavailable")
	}
	input, err := os.Open(dump)
	if err != nil {
		return BackupRestoreResult{}, err
	}
	defer input.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := helperCommandContext(ctx, cfg, cfg.DBCtl, "restore-dump", database, site)
	cmd.Stdin = input
	output, err := runBoundedCommand(ctx, cmd)
	if err != nil {
		return BackupRestoreResult{}, fmt.Errorf("restore database %s: %w: %s", database, err, strings.TrimSpace(string(output)))
	}
	return BackupRestoreResult{Site: site, Backup: filepath.Base(backup), Mode: "database-only", Database: database, DatabaseRestored: true, DatabasePreserved: false, Consistency: manifest.Consistency, SchemaRollback: "manual: restore the safety backup or apply a forward migration", CompletedAt: time.Now().UTC()}, nil
}

func (a *App) backupRestoreDatabaseHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) || !a.Auth.IsAdministrator(r) {
		http.Error(w, "administrator CSRF request required", http.StatusForbidden)
		return
	}
	var input struct {
		Backup   string `json:"backup"`
		Site     string `json:"site"`
		Database string `json:"database"`
		Confirm  string `json:"confirm"`
	}
	if err := decodeJSON(w, r, 4096, &input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	input.Site = safeUser(input.Site)
	input.Backup = filepath.Base(strings.TrimSpace(input.Backup))
	if input.Site == "" || input.Backup == "" || input.Backup == "." || !validManagedDatabaseIdentifier(input.Database, 64) || input.Confirm != "RESTORE_DATABASE" {
		http.Error(w, "site, backup, database, and confirm=RESTORE_DATABASE are required", http.StatusUnprocessableEntity)
		return
	}
	if a.Jobs == nil || a.Config.DBCtl == "" {
		http.Error(w, "managed database restore is unavailable", http.StatusServiceUnavailable)
		return
	}
	manifest, err := VerifySiteBackup(filepath.Join(a.Config.BackupRoot, input.Backup), a.Config.BackupSigningKey)
	if err != nil || manifest.Site != input.Site || !backupContainsDatabase(manifest, input.Database) {
		http.Error(w, "verified backup does not contain the selected site database", http.StatusUnprocessableEntity)
		return
	}
	jobID, err := newJobID("backup-db-restore")
	if err != nil {
		http.Error(w, "could not create restore job", http.StatusInternalServerError)
		return
	}
	if err := a.Jobs.SubmitBackupRestore(jobID, input.Site, func() (BackupRestoreResult, error) {
		safety, safetyErr := CreateSiteBackup(a.Config, input.Site, true)
		if safetyErr != nil {
			return BackupRestoreResult{}, fmt.Errorf("create pre-restore safety backup: %w", safetyErr)
		}
		result, restoreErr := restoreManagedDatabase(a.Config, input.Backup, input.Site, input.Database)
		if restoreErr != nil {
			_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "backup.restore-database.failed", input.Site, restoreErr.Error())
			return result, restoreErr
		}
		result.SafetyBackup = safety.Path
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "backup.restore-database.completed", input.Site, input.Database+" safety_backup="+safety.Path)
		return result, nil
	}); err != nil {
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		} else {
			http.Error(w, "could not persist restore job", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status_url": "/api/jobs/" + jobID, "mode": "database-only", "schema_rollback": "manual"})
}
