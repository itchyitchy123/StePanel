package main

import (
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
