package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentStoreRequiresKeyForSecretState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "environment.json")
	store, err := OpenEnvironmentStore(path, "test-environment-key")
	if err != nil {
		t.Fatal(err)
	}
	store.values["demo"] = map[string]environmentValue{
		"APP_KEY": {Value: "secret", Secret: true},
	}
	store.mu.Lock()
	err = store.persistLocked()
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenEnvironmentStore(path, ""); err == nil || !strings.Contains(err.Error(), "encryption key") {
		t.Fatalf("OpenEnvironmentStore without key error = %v; want encryption-key error", err)
	}
}

func TestEnvironmentStoreEncryptWithoutKeyReturnsError(t *testing.T) {
	store := &EnvironmentStore{}
	if _, err := store.encrypt("secret"); err == nil {
		t.Fatal("encrypt without a key unexpectedly succeeded")
	}
}
