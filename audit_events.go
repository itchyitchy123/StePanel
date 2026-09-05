package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// auditEvents exposes a bounded, filtered audit view for operators. The
// underlying chain is verified before records are returned so dashboards do
// not accidentally present tampered deployment history as trustworthy.
func (a *App) auditEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			http.Error(w, "limit must be between 1 and 500", http.StatusUnprocessableEntity)
			return
		}
		limit = value
	}
	path := a.Config.AuditLog
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{"events": []AuditEvent{}, "integrity": "unconfigured"})
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		writeJSON(w, http.StatusOK, map[string]any{"events": []AuditEvent{}, "integrity": "empty"})
		return
	}
	if err := VerifyAuditLog(path); err != nil {
		http.Error(w, "audit integrity verification failed", http.StatusServiceUnavailable)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.Error(w, "unable to read audit log", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	target, action := r.URL.Query().Get("target"), r.URL.Query().Get("action")
	events := make([]AuditEvent, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var event AuditEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			http.Error(w, "audit log contains invalid JSON", http.StatusServiceUnavailable)
			return
		}
		if target != "" && !strings.EqualFold(target, event.Target) || action != "" && !strings.EqualFold(action, event.Action) {
			continue
		}
		events = append(events, event)
		if len(events) > limit {
			events = events[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		http.Error(w, "unable to scan audit log", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "integrity": "verified"})
}
