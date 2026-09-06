package main

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StagingRequest struct {
	Source       string `json:"source"`
	Site         string `json:"site"`
	Domain       string `json:"domain"`
	Files        bool   `json:"files"`
	Environment  bool   `json:"environment"`
	Database     bool   `json:"database"`
	NoIndex      *bool  `json:"no_index,omitempty"`
	BasicAuth    *bool  `json:"basic_auth,omitempty"`
	AuthUser     string `json:"auth_user,omitempty"`
	AuthPassword string `json:"auth_password,omitempty"`
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
	marker := filepath.Join(a.Config.WebRoot, "sites", input.Site, ".stepanel-staging-noindex")
	previousMarker, markerErr := os.ReadFile(marker)
	if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
		http.Error(w, "could not inspect staging indexing protection", 503)
		return
	}
	markerExisted := markerErr == nil
	var previousEnvironment map[string]environmentValue
	previousEnvironmentExists := false
	if input.Environment && a.Environments != nil {
		a.Environments.mu.RLock()
		if values, exists := a.Environments.values[input.Site]; exists {
			previousEnvironmentExists = true
			previousEnvironment = cloneEnvironmentValues(values)
		}
		a.Environments.mu.RUnlock()
	}
	routeExtension := ".conf"
	if a.Config.WebServer == "caddy" {
		routeExtension = ".caddy"
	}
	routeName := "site-" + input.Site + "-" + strings.ReplaceAll(input.Domain, ".", "_") + routeExtension
	routeApplied := false
	stateRollback := func() {
		if routeApplied {
			if err := runHelperCommand(r.Context(), a.Config, a.Config.VHostCtl, "delete", routeName); err != nil {
				log.Printf("staging route cleanup failed for %s: %v", input.Site, err)
			}
		}
		if input.Environment && a.Environments != nil {
			a.Environments.mu.Lock()
			if previousEnvironmentExists {
				a.Environments.values[input.Site] = previousEnvironment
			} else {
				delete(a.Environments.values, input.Site)
			}
			err := a.Environments.persistLocked()
			a.Environments.mu.Unlock()
			if err != nil {
				log.Printf("staging environment rollback failed for %s: %v", input.Site, err)
			}
		}
		if markerExisted {
			if err := writeAtomic(marker, previousMarker, 0600); err != nil {
				log.Printf("staging marker rollback failed for %s: %v", input.Site, err)
			}
		} else if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("staging marker cleanup failed for %s: %v", input.Site, err)
		}
	}
	stateCommitted := false
	defer func() {
		if !stateCommitted {
			stateRollback()
		}
	}()
	if err := siteHelper(a.Config, "prepare", input.Site); err != nil {
		http.Error(w, "could not prepare staging site", 502)
		return
	}
	noIndex := input.NoIndex == nil || *input.NoIndex
	basicAuth := input.BasicAuth != nil && *input.BasicAuth
	authHash := ""
	if basicAuth {
		input.AuthUser = strings.TrimSpace(input.AuthUser)
		if len(input.AuthUser) < 1 || len(input.AuthUser) > 64 || strings.ContainsAny(input.AuthUser, "\x00\r\n:") || len(input.AuthPassword) < 12 || len(input.AuthPassword) > 256 || strings.ContainsAny(input.AuthPassword, "\x00\r\n") {
			http.Error(w, "valid Basic Auth username and password are required", 422)
			return
		}
		var err error
		hash, err := bcrypt.GenerateFromPassword([]byte(input.AuthPassword), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "could not hash staging credentials", 500)
			return
		}
		authHash = string(hash)
		input.AuthPassword = ""
	}
	if noIndex {
		if err := writeAtomic(marker, []byte("managed staging noindex\n"), 0600); err != nil {
			http.Error(w, "could not apply staging indexing protection", 503)
			return
		}
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
	vhostAction := "apply"
	if basicAuth {
		vhostAction = "apply-auth"
	}
	args := []string{vhostAction, input.Site, input.Domain}
	if basicAuth {
		args = append(args, input.AuthUser, authHash)
	}
	if err := runHelperCommand(r.Context(), a.Config, a.Config.VHostCtl, args...); err != nil {
		http.Error(w, "could not activate staging route", 502)
		return
	}
	routeApplied = true
	if err := txn.Commit(); err != nil {
		http.Error(w, "could not commit staging transaction", 503)
		return
	}
	ok = true
	stateCommitted = true
	result := StagingResult{Source: input.Source, Site: input.Site, Domain: input.Domain, FilesCopied: input.Files, EnvironmentCopied: input.Environment, SecretsCopied: false, CreatedAt: time.Now().UTC()}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "staging.created", input.Site, input.Source+" -> "+input.Domain)
	writeJSON(w, 202, result)
}
