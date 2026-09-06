package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

type SSHKey struct {
	Label       string `json:"label"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}
type SiteAccess struct {
	Site         string   `json:"site"`
	SFTPEnabled  bool     `json:"sftp_enabled"`
	ShellEnabled bool     `json:"shell_enabled"`
	Keys         []SSHKey `json:"keys"`
}
type SiteAccessStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]SiteAccess
}

func OpenSiteAccessStore(path string) (*SiteAccessStore, error) {
	s := &SiteAccessStore{path: path, values: map[string]SiteAccess{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &s.values); err != nil {
		return nil, fmt.Errorf("decode SSH access state: %w", err)
	}
	return s, nil
}
func (s *SiteAccessStore) persistLocked() error {
	data, err := json.MarshalIndent(s.values, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(data, '\n'), 0600)
}
func validateSSHLabel(label string) bool {
	if label == "" || len(label) > 64 {
		return false
	}
	for _, c := range label {
		if !(c == '-' || c == '_' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
func parseSSHKey(raw string) (SSHKey, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(raw)))
	if err != nil {
		return SSHKey{}, errors.New("invalid SSH public key")
	}
	typ := key.Type()
	if typ == "ssh-dss" {
		return SSHKey{}, errors.New("DSA SSH keys are not permitted")
	}
	return SSHKey{PublicKey: strings.TrimSpace(raw), Fingerprint: ssh.FingerprintSHA256(key)}, nil
}

func (a *App) siteAccess(w http.ResponseWriter, r *http.Request) {
	site := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/access/"), "/")
	if site == "" || strings.Contains(site, "/") || safeUser(site) == "" {
		http.Error(w, "invalid site", 422)
		return
	}
	if !a.canAccessSite(r, site) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	if a.Access == nil {
		http.Error(w, "SSH access state unavailable", 503)
		return
	}
	a.Access.mu.RLock()
	access := a.Access.values[site]
	if access.Site == "" {
		access = SiteAccess{Site: site, SFTPEnabled: true, Keys: []SSHKey{}}
	}
	a.Access.mu.RUnlock()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, access)
	case http.MethodPatch:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		var input struct {
			SFTPEnabled  *bool `json:"sftp_enabled"`
			ShellEnabled *bool `json:"shell_enabled"`
		}
		if err := decodeJSON(w, r, 2048, &input); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if input.SFTPEnabled != nil {
			access.SFTPEnabled = *input.SFTPEnabled
		}
		if input.ShellEnabled != nil {
			access.ShellEnabled = *input.ShellEnabled
		}
		access.Site = site
		a.Access.mu.Lock()
		a.Access.values[site] = access
		err := a.Access.persistLocked()
		a.Access.mu.Unlock()
		if err != nil {
			http.Error(w, "SSH access state could not be saved", 503)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.ssh-access.updated", site, "access policy changed")
		writeJSON(w, 200, access)
	case http.MethodPost:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		var input struct {
			Label     string `json:"label"`
			PublicKey string `json:"public_key"`
		}
		if err := decodeJSON(w, r, 8<<10, &input); err != nil || !validateSSHLabel(input.Label) {
			http.Error(w, "invalid key label", 422)
			return
		}
		key, err := parseSSHKey(input.PublicKey)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		key.Label = input.Label
		for _, existing := range access.Keys {
			if existing.Label == key.Label || existing.Fingerprint == key.Fingerprint {
				http.Error(w, "SSH key label or fingerprint already exists", 409)
				return
			}
		}
		access.Site = site
		access.Keys = append(access.Keys, key)
		a.Access.mu.Lock()
		a.Access.values[site] = access
		err = a.Access.persistLocked()
		a.Access.mu.Unlock()
		if err != nil {
			http.Error(w, "SSH key could not be saved", 503)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.ssh-key.added", site, key.Fingerprint)
		writeJSON(w, 201, key)
	}
}

func (a *App) siteAccessKey(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/access/"), "/"), "/")
	if len(parts) != 2 {
		http.Error(w, "invalid site or key label", 422)
		return
	}
	site, label := parts[0], parts[1]
	if !a.canAccessSite(r, site) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid CSRF token", 403)
		return
	}
	a.Access.mu.Lock()
	access, ok := a.Access.values[site]
	if !ok {
		a.Access.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	found := false
	keys := access.Keys[:0]
	for _, key := range access.Keys {
		if key.Label == label {
			found = true
		} else {
			keys = append(keys, key)
		}
	}
	access.Keys = keys
	access.Site = site
	if !found {
		a.Access.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	a.Access.values[site] = access
	err := a.Access.persistLocked()
	a.Access.mu.Unlock()
	if err != nil {
		http.Error(w, "SSH key could not be removed", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.ssh-key.removed", site, label)
	w.WriteHeader(204)
}

func siteAccessStatePath(cfg Config) string {
	return filepath.Join(filepath.Dir(cfg.JobState), "site-access.json")
}
