// Package session owns durable, revocable session-registry state.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Username string `json:"username"`
	Expiry   int64  `json:"expiry"`
}

type Registry struct {
	mu      sync.RWMutex
	path    string
	Entries map[string]Entry
	err     error
}

// New creates an empty registry. It is useful when the caller needs to build
// state before the first persistence operation, while Open is preferred for
// normal startup.
func New(path string) *Registry {
	return &Registry{path: path, Entries: make(map[string]Entry)}
}

func Open(path string) (*Registry, error) {
	registry := &Registry{path: path, Entries: make(map[string]Entry)}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) > 1<<20 {
			return nil, errors.New("session state exceeds 1 MiB")
		}
		if err := json.Unmarshal(data, &registry.Entries); err != nil {
			var legacy map[string]int64
			if legacyErr := json.Unmarshal(data, &legacy); legacyErr != nil {
				return nil, err
			}
			for id, expiry := range legacy {
				registry.Entries[id] = Entry{Expiry: expiry}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	now := time.Now().Unix()
	for id, entry := range registry.Entries {
		if id == "" || entry.Expiry <= now {
			delete(registry.Entries, id)
		}
	}
	if err := registry.persistLocked(); err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *Registry) Add(id, username string, expiry int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().Unix()
	for candidate, entry := range r.Entries {
		if entry.Expiry <= now {
			delete(r.Entries, candidate)
		}
	}
	if len(r.Entries) >= 10000 {
		oldestID := ""
		var oldest int64
		for candidate, entry := range r.Entries {
			if oldestID == "" || entry.Expiry < oldest {
				oldestID, oldest = candidate, entry.Expiry
			}
		}
		if oldestID != "" {
			delete(r.Entries, oldestID)
		}
	}
	r.Entries[id] = Entry{Username: username, Expiry: expiry}
	if err := r.persistLocked(); err != nil {
		delete(r.Entries, id)
		r.err = err
		return err
	}
	r.err = nil
	return nil
}

func (r *Registry) Valid(id, username string, expiry int64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.Entries[id]
	return ok && entry.Expiry == expiry && entry.Username == username && expiry > time.Now().Unix()
}

func (r *Registry) RevokeUser(username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := make(map[string]Entry, len(r.Entries))
	for id, entry := range r.Entries {
		previous[id] = entry
	}
	for id, entry := range r.Entries {
		if entry.Username == username {
			delete(r.Entries, id)
		}
	}
	if err := r.persistLocked(); err != nil {
		r.Entries = previous
		r.err = err
		return err
	}
	r.err = nil
	return nil
}

func (r *Registry) Revoke(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous, existed := r.Entries[id]
	delete(r.Entries, id)
	if err := r.persistLocked(); err != nil {
		if existed {
			r.Entries[id] = previous
		}
		r.err = err
		return err
	}
	r.err = nil
	return nil
}

func (r *Registry) PersistenceError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.err
}

func (r *Registry) persistLocked() error {
	if r.path == "" {
		return nil
	}
	data, err := json.Marshal(r.Entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".session-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, r.path)
}
