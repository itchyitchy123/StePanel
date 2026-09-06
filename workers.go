package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Worker struct {
	Site      string `json:"site"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Processes int    `json:"processes"`
	MemoryMB  int    `json:"memory_mb"`
	Retries   int    `json:"retries"`
	Root      string `json:"root"`
}
type WorkerStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]Worker
}

var workerTypes = map[string]bool{"laravel": true, "horizon": true, "node": true, "celery": true, "rq": true}

func OpenWorkerStore(path string) (*WorkerStore, error) {
	s := &WorkerStore{path: path, values: map[string]Worker{}}
	d, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(d, &s.values); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *WorkerStore) persistLocked() error {
	d, e := json.MarshalIndent(s.values, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(s.path, append(d, '\n'), 0600)
}
func validWorkerName(v string) bool {
	if v == "" || len(v) > 32 {
		return false
	}
	for _, c := range v {
		if !(c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
func (a *App) workers(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workers/"), "/"), "/")
	if len(parts) < 1 || safeUser(parts[0]) == "" || !a.canAccessSite(r, parts[0]) {
		http.Error(w, "invalid or inaccessible site", 403)
		return
	}
	site := parts[0]
	name := ""
	if len(parts) > 1 {
		name = parts[1]
	}
	if a.Workers == nil {
		http.Error(w, "worker state unavailable", 503)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		a.Workers.mu.RLock()
		list := []Worker{}
		for _, v := range a.Workers.values {
			if v.Site == site {
				list = append(list, v)
			}
		}
		a.Workers.mu.RUnlock()
		writeJSON(w, 200, map[string]any{"workers": list})
		return
	}
	if len(parts) == 3 && r.Method == http.MethodPost {
		if !a.Auth.CSRF(r) || (parts[2] != "start" && parts[2] != "stop" && parts[2] != "restart") {
			http.Error(w, "invalid worker action", 422)
			return
		}
		if err := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, parts[2], site+"/"+name); err != nil {
			http.Error(w, "worker action failed", 502)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "worker."+parts[2], site, name)
		writeJSON(w, 202, map[string]string{"site": site, "name": name, "action": parts[2]})
		return
	}
	if len(parts) != 2 {
		http.Error(w, "invalid worker", 422)
		return
	}
	name = parts[1]
	if r.Method == http.MethodDelete {
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		a.Workers.mu.Lock()
		delete(a.Workers.values, site+"/"+name)
		e := a.Workers.persistLocked()
		a.Workers.mu.Unlock()
		if e != nil {
			http.Error(w, "worker state could not be saved", 503)
			return
		}
		_ = runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "worker-delete", site, name)
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPut || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var input Worker
	if e := decodeJSON(w, r, 4096, &input); e != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	input.Site = site
	input.Name = name
	if !validWorkerName(name) || !workerTypes[input.Type] || input.Processes < 1 || input.Processes > 64 || input.MemoryMB < 64 || input.MemoryMB > 65536 || input.Retries < 0 || input.Retries > 20 {
		http.Error(w, "invalid worker definition", 422)
		return
	}
	input.Root = filepath.Join(a.Config.WebRoot, "sites", site, "public")
	if e := ensureInside(a.Config.WebRoot, input.Root); e != nil {
		http.Error(w, "invalid worker root", 422)
		return
	}
	if e := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "worker-apply", site, name, input.Type, input.Root, strconv.Itoa(input.Processes), strconv.Itoa(input.MemoryMB), strconv.Itoa(input.Retries)); e != nil {
		http.Error(w, "worker helper failed", 502)
		return
	}
	a.Workers.mu.Lock()
	a.Workers.values[site+"/"+name] = input
	e := a.Workers.persistLocked()
	a.Workers.mu.Unlock()
	if e != nil {
		http.Error(w, "worker state could not be saved", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "worker.updated", site, name)
	writeJSON(w, 202, input)
}
