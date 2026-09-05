package main

import (
	"bufio"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
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
	target, action := r.URL.Query().Get("target"), r.URL.Query().Get("action")
	events, err := readVerifiedAuditEvents(path, target, action, limit)
	if err != nil {
		http.Error(w, "audit integrity verification failed", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "integrity": "verified"})
}

// readVerifiedAuditEvents verifies and filters in one pass. This avoids the
// previous full-file verification followed by a second full-file scan.
func readVerifiedAuditEvents(path, target, action string, limit int) ([]AuditEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	events := make([]AuditEvent, 0, limit)
	var first, previous *AuditEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		if err := validateAuditEvent(event); err != nil {
			return nil, err
		}
		expected, err := hashAuditEvent(event)
		if err != nil || !hmac.Equal([]byte(expected), []byte(event.Hash)) {
			return nil, fmt.Errorf("audit event %d has an invalid signature", event.Sequence)
		}
		if previous != nil && (event.Sequence != previous.Sequence+1 || event.PreviousHash != previous.Hash) {
			return nil, fmt.Errorf("audit chain breaks at event %d", event.Sequence)
		}
		copyEvent := event
		if first == nil {
			first = &copyEvent
		}
		previous = &copyEvent
		if target == "" || strings.EqualFold(target, event.Target) {
			if action == "" || strings.EqualFold(action, event.Action) {
				events = append(events, event)
				if len(events) > limit {
					events = events[1:]
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if previous == nil {
		return nil, errors.New("audit log contains no signed events")
	}
	state, err := loadAuditState(path + ".state")
	if err != nil {
		return nil, err
	}
	if state.Sequence != previous.Sequence || state.Hash != previous.Hash || first.Sequence != state.FirstSequence || first.PreviousHash != state.FirstPreviousHash {
		return nil, errors.New("audit chain state does not match the log")
	}
	return events, nil
}
