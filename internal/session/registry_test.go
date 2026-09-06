package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryPersistsAndRevokesByUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	registry, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour).Unix()
	if err := registry.Add("one", "alice", expiry); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("two", "bob", expiry); err != nil {
		t.Fatal(err)
	}
	if !registry.Valid("one", "alice", expiry) || registry.Valid("one", "bob", expiry) {
		t.Fatal("session validation did not enforce identity")
	}
	if err := registry.RevokeUser("alice"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Valid("one", "alice", expiry) || !reopened.Valid("two", "bob", expiry) {
		t.Fatal("user revocation was not persisted")
	}
}
