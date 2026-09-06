package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StagingRequest struct {
	Source      string `json:"source"`
	Site        string `json:"site"`
	Domain      string `json:"domain"`
	Files       bool   `json:"files"`
	Environment bool   `json:"environment"`
	Database    bool   `json:"database"`
}
type StagingResult struct {
	Source            string    `json:"source"`
	Site              string    `json:"site"`
	Domain            string    `json:"domain"`
	FilesCopied       bool      `json:"files_copied"`
	EnvironmentCopied bool      `json:"environment_copied"`
	SecretsCopied     bool      `json:"secrets_copied"`
	CreatedAt         time.Time `json:"created_at"`
}

func (a *App) stagingCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var input StagingRequest
	if err := decodeJSON(w, r, 4096, &input); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	input.Source = safeUser(input.Source)
	input.Site = safeUser(input.Site)
	input.Domain = strings.ToLower(strings.TrimSpace(input.Domain))
	if input.Source == "" || input.Site == "" || input.Source == input.Site || !domainPattern.MatchString(input.Domain) {
		http.Error(w, "invalid staging source, site, or domain", 422)
		return
	}
	if !a.canAccessSite(r, input.Source) || !a.canAccessSite(r, input.Site) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	if input.Database {
		http.Error(w, "database cloning requires the transactional managed-database clone helper", 422)
		return
	}
	source := filepath.Join(a.Config.WebRoot, "sites", input.Source, "public")
	dest := filepath.Join(a.Config.WebRoot, "sites", input.Site, "public")
	if err := ensureInside(a.Config.WebRoot, source); err != nil {
		http.Error(w, "invalid source", 422)
		return
	}
	if _, err := os.Stat(source); err != nil {
		http.Error(w, "source site does not exist", 422)
		return
	}
	if err := siteHelper(a.Config, "prepare", input.Site); err != nil {
		http.Error(w, "could not prepare staging site", 502)
		return
	}
	txn, err := BeginSiteTransaction(a.Config.RecoveryRoot, dest, "staging.clone", input.Site)
	if err != nil {
		http.Error(w, "could not begin staging transaction", 503)
		return
	}
	ok := false
	defer func() {
		if !ok {
			_ = txn.Rollback()
		}
	}()
	if input.Files {
		if err := copyTree(source, dest); err != nil {
			http.Error(w, "copy staging files: "+err.Error(), 502)
			return
		}
	}
	if input.Environment && a.Environments != nil {
		a.Environments.mu.RLock()
		vars := a.Environments.values[input.Source]
		copied := map[string]environmentValue{}
		for name, value := range vars {
			if !value.Secret {
				copied[name] = value
			}
		}
		a.Environments.mu.RUnlock()
		a.Environments.mu.Lock()
		a.Environments.values[input.Site] = copied
		err = a.Environments.persistLocked()
		a.Environments.mu.Unlock()
		if err != nil {
			http.Error(w, "could not persist staging environment", 503)
			return
		}
	}
	if err := siteHelper(a.Config, "seal", input.Site); err != nil {
		http.Error(w, "could not seal staging site", 502)
		return
	}
	if err := runHelperCommand(r.Context(), a.Config, a.Config.VHostCtl, "apply", input.Site, input.Domain); err != nil {
		http.Error(w, "could not activate staging route", 502)
		return
	}
	if err := txn.Commit(); err != nil {
		http.Error(w, "could not commit staging transaction", 503)
		return
	}
	ok = true
	result := StagingResult{Source: input.Source, Site: input.Site, Domain: input.Domain, FilesCopied: input.Files, EnvironmentCopied: input.Environment, SecretsCopied: false, CreatedAt: time.Now().UTC()}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "staging.created", input.Site, input.Source+" -> "+input.Domain)
	writeJSON(w, 202, result)
}
