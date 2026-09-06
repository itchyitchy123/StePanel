package main

import (
	"context"
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
	State     string `json:"state,omitempty"`
	LastError string `json:"last_error,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
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
	for key, worker := range s.values {
		if worker.State == "" {
			worker.State = "applied"
		}
		s.values[key] = worker
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
			if v.Site == site && !v.Deleted {
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
		key := site + "/" + name
		a.Workers.mu.Lock()
		worker := a.Workers.values[key]
		worker.Site, worker.Name = site, name
		worker.State, worker.LastError, worker.Deleted = "pending", "", true
		a.Workers.values[key] = worker
		e := a.Workers.persistLocked()
		a.Workers.mu.Unlock()
		if e != nil {
			http.Error(w, "worker state could not be saved", 503)
			return
		}
		if e := a.applyWorker(r.Context(), worker); e != nil {
			a.recordWorkerError(key, e)
			http.Error(w, "worker removal is pending reconciliation", http.StatusBadGateway)
			return
		}
		a.Workers.mu.Lock()
		delete(a.Workers.values, key)
		e = a.Workers.persistLocked()
		if e != nil {
			a.Workers.values[key] = worker
		}
		a.Workers.mu.Unlock()
		if e != nil {
			http.Error(w, "worker removed but state cleanup is pending", 503)
			return
		}
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
	input.State, input.LastError, input.Deleted = "pending", "", false
	if !validWorkerName(name) || !workerTypes[input.Type] || input.Processes < 1 || input.Processes > 64 || input.MemoryMB < 64 || input.MemoryMB > 65536 || input.Retries < 0 || input.Retries > 20 {
		http.Error(w, "invalid worker definition", 422)
		return
	}
	input.Root = filepath.Join(a.Config.WebRoot, "sites", site, "public")
	if e := ensureInside(a.Config.WebRoot, input.Root); e != nil {
		http.Error(w, "invalid worker root", 422)
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
	if e := a.applyWorker(r.Context(), input); e != nil {
		a.recordWorkerError(site+"/"+name, e)
		http.Error(w, "worker is pending reconciliation", 502)
		return
	}
	input.State, input.LastError = "applied", ""
	a.Workers.mu.Lock()
	a.Workers.values[site+"/"+name] = input
	e = a.Workers.persistLocked()
	a.Workers.mu.Unlock()
	if e != nil {
		a.recordWorkerError(site+"/"+name, e)
		http.Error(w, "worker applied but state update is pending", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "worker.updated", site, name)
	writeJSON(w, 202, input)
}

func (a *App) applyWorker(ctx context.Context, worker Worker) error {
	if worker.Deleted {
		return runHelperCommand(ctx, a.Config, a.Config.AppCtl, "worker-delete", worker.Site, worker.Name)
	}
	return runHelperCommand(ctx, a.Config, a.Config.AppCtl, "worker-apply", worker.Site, worker.Name, worker.Type, worker.Root, strconv.Itoa(worker.Processes), strconv.Itoa(worker.MemoryMB), strconv.Itoa(worker.Retries))
}

func (a *App) recordWorkerError(key string, applyErr error) {
	a.Workers.mu.Lock()
	defer a.Workers.mu.Unlock()
	if worker, ok := a.Workers.values[key]; ok {
		worker.State = "pending"
		worker.LastError = applyErr.Error()
		a.Workers.values[key] = worker
		_ = a.Workers.persistLocked()
	}
}

func (a *App) reconcileWorkers(ctx context.Context) (reconciled []string, failed map[string]string) {
	failed = map[string]string{}
	if a.Workers == nil {
		return nil, failed
	}
	a.Workers.mu.RLock()
	pending := make([]Worker, 0)
	for _, worker := range a.Workers.values {
		if worker.State == "pending" || worker.Deleted {
			pending = append(pending, worker)
		}
	}
	a.Workers.mu.RUnlock()
	for _, worker := range pending {
		key := worker.Site + "/" + worker.Name
		if err := a.applyWorker(ctx, worker); err != nil {
			failed[key] = err.Error()
			a.recordWorkerError(key, err)
			continue
		}
		a.Workers.mu.Lock()
		if worker.Deleted {
			delete(a.Workers.values, key)
		} else {
			worker.State, worker.LastError = "applied", ""
			a.Workers.values[key] = worker
		}
		err := a.Workers.persistLocked()
		if err != nil {
			worker.State, worker.LastError = "pending", "state persistence failed"
			a.Workers.values[key] = worker
		}
		a.Workers.mu.Unlock()
		if err != nil {
			failed[key] = "state persistence failed"
			continue
		}
		reconciled = append(reconciled, key)
	}
	return reconciled, failed
}
