package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

type environmentValue struct {
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}
type EnvironmentStore struct {
	mu     sync.RWMutex
	path   string
	key    []byte
	values map[string]map[string]environmentValue
}

func OpenEnvironmentStore(path, secret string) (*EnvironmentStore, error) {
	if strings.TrimSpace(secret) == "" {
		return &EnvironmentStore{path: path, values: map[string]map[string]environmentValue{}}, nil
	}
	h := sha256.Sum256([]byte(secret))
	store := &EnvironmentStore{path: path, key: h[:], values: map[string]map[string]environmentValue{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &store.values); err != nil {
		return nil, fmt.Errorf("decode environment state: %w", err)
	}
	for site, vars := range store.values {
		for name, value := range vars {
			if value.Secret {
				plain, err := store.decrypt(value.Value)
				if err != nil {
					return nil, fmt.Errorf("decrypt %s/%s: %w", site, name, err)
				}
				value.Value = plain
				vars[name] = value
			}
		}
	}
	return store, nil
}

func (s *EnvironmentStore) encrypt(value string) (string, error) {
	block, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}
func (s *EnvironmentStore) decrypt(value string) (string, error) {
	block, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(block)
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted value")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}
func (s *EnvironmentStore) persistLocked() error {
	out := make(map[string]map[string]environmentValue, len(s.values))
	for site, vars := range s.values {
		out[site] = map[string]environmentValue{}
		for name, value := range vars {
			if value.Secret {
				encrypted, err := s.encrypt(value.Value)
				if err != nil {
					return err
				}
				value.Value = encrypted
			}
			out[site][name] = value
		}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(data, '\n'), 0600)
}

func (a *App) siteEnvironment(w http.ResponseWriter, r *http.Request) {
	site := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/environment/"), "/")
	if site == "" || strings.Contains(site, "/") || safeUser(site) == "" {
		http.Error(w, "invalid site", 422)
		return
	}
	if !a.canAccessSite(r, site) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	if a.Environments == nil || len(a.Environments.key) == 0 {
		http.Error(w, "environment management is not configured", 503)
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.Environments.mu.RLock()
		vars := a.Environments.values[site]
		result := map[string]any{}
		for name, value := range vars {
			if value.Secret {
				result[name] = map[string]any{"secret": true, "configured": value.Value != ""}
			} else {
				result[name] = value.Value
			}
		}
		a.Environments.mu.RUnlock()
		writeJSON(w, 200, map[string]any{"site": site, "environment": result})
	case http.MethodPut:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		var input map[string]environmentValue
		if err := decodeJSON(w, r, 64<<10, &input); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		for name := range input {
			if !validEnvName(name) {
				http.Error(w, "invalid environment variable name", 422)
				return
			}
		}
		a.Environments.mu.Lock()
		a.Environments.values[site] = input
		err := a.Environments.persistLocked()
		a.Environments.mu.Unlock()
		if err != nil {
			http.Error(w, "environment state could not be saved", 503)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.environment.updated", site, fmt.Sprintf("%d variables", len(input)))
		w.WriteHeader(204)
	case http.MethodDelete:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		a.Environments.mu.Lock()
		delete(a.Environments.values, site)
		err := a.Environments.persistLocked()
		a.Environments.mu.Unlock()
		if err != nil {
			http.Error(w, "environment state could not be saved", 503)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "site.environment.deleted", site, "all variables removed")
		w.WriteHeader(204)
	}
}

func validEnvName(name string) bool {
	if name == "" || len(name) > 128 || name[0] == '=' {
		return false
	}
	for _, c := range name {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
