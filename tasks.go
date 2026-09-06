package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ScheduledTask is intentionally a systemd-timer definition, rather than a
// writable crontab fragment. Commands execute as the isolated site identity;
// the helper applies time, process and filesystem restrictions consistently.
type ScheduledTask struct {
	Site       string `json:"site"`
	Name       string `json:"name"`
	Runtime    string `json:"runtime"`
	Command    string `json:"command"`
	OnCalendar string `json:"on_calendar"`
	TimeoutSec int    `json:"timeout_sec"`
	Enabled    bool   `json:"enabled"`
}

type TaskStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]ScheduledTask
}

func OpenTaskStore(path string) (*TaskStore, error) {
	s := &TaskStore{path: path, values: map[string]ScheduledTask{}}
	d, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(d, &s.values); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *TaskStore) persistLocked() error {
	d, err := json.MarshalIndent(s.values, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(d, '\n'), 0600)
}
func validTaskRuntime(v string) bool {
	return v == "php" || v == "node" || v == "python" || v == "shell"
}

func (a *App) tasks(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/"), "/")
	if len(parts) < 1 || safeUser(parts[0]) == "" || !a.canAccessSite(r, parts[0]) || a.Tasks == nil {
		http.Error(w, "invalid or inaccessible site", 403)
		return
	}
	site := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		a.Tasks.mu.RLock()
		result := []ScheduledTask{}
		for _, task := range a.Tasks.values {
			if task.Site == site {
				result = append(result, task)
			}
		}
		a.Tasks.mu.RUnlock()
		writeJSON(w, 200, map[string]any{"tasks": result})
		return
	}
	if len(parts) != 2 || !validWorkerName(parts[1]) {
		http.Error(w, "invalid task", 422)
		return
	}
	name := parts[1]
	if r.Method == http.MethodDelete {
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid CSRF token", 403)
			return
		}
		if err := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "task-delete", site, name); err != nil {
			http.Error(w, "could not remove scheduled task", 502)
			return
		}
		a.Tasks.mu.Lock()
		delete(a.Tasks.values, site+"/"+name)
		err := a.Tasks.persistLocked()
		a.Tasks.mu.Unlock()
		if err != nil {
			http.Error(w, "could not persist task state", 503)
			return
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "task.deleted", site, name)
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPut || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", 403)
		return
	}
	var input ScheduledTask
	if err := decodeJSON(w, r, 8192, &input); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	input.Site, input.Name = site, name
	input.Command, input.OnCalendar = strings.TrimSpace(input.Command), strings.TrimSpace(input.OnCalendar)
	if !validTaskRuntime(input.Runtime) || input.Command == "" || len(input.Command) > 1024 || strings.ContainsAny(input.Command, "\x00\r\n") || input.OnCalendar == "" || len(input.OnCalendar) > 128 || strings.ContainsAny(input.OnCalendar, "\x00\r\n") || input.TimeoutSec < 1 || input.TimeoutSec > 86400 {
		http.Error(w, "invalid scheduled task definition", 422)
		return
	}
	encodedCommand := base64.RawStdEncoding.EncodeToString([]byte(input.Command))
	if err := runHelperCommand(r.Context(), a.Config, a.Config.AppCtl, "task-apply", site, name, input.Runtime, input.OnCalendar, stringBool(input.Enabled), itoa(input.TimeoutSec), encodedCommand); err != nil {
		http.Error(w, "scheduled task helper rejected the definition", 502)
		return
	}
	a.Tasks.mu.Lock()
	a.Tasks.values[site+"/"+name] = input
	err := a.Tasks.persistLocked()
	a.Tasks.mu.Unlock()
	if err != nil {
		http.Error(w, "could not persist task state", 503)
		return
	}
	_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "task.updated", site, name)
	writeJSON(w, 202, input)
}

func stringBool(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
