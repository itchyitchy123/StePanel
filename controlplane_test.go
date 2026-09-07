package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlPlaneStateBlobIsTransactionalAndPersistent(t *testing.T) {
	db, err := openControlPlaneDB(filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	type state struct {
		Owner string   `json:"owner"`
		Sites []string `json:"sites"`
	}
	store := &struct{}{}
	value := state{Owner: "customer", Sites: []string{"site-a"}}
	found, err := bindControlPlaneState(store, db, "test-state", &value)
	if err != nil || found {
		t.Fatalf("initial state binding = found %v err %v", found, err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if bound, err := persistBoundControlPlaneState(store, payload); !bound || err != nil {
		t.Fatalf("persist state = bound %v err %v", bound, err)
	}
	value = state{}
	found, err = bindControlPlaneState(&struct{}{}, db, "test-state", &value)
	if err != nil || !found || value.Owner != "customer" || len(value.Sites) != 1 {
		t.Fatalf("reloaded state = %#v found %v err %v", value, found, err)
	}
}

func TestControlPlaneBackupCanBeVerified(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "control-plane.db")
	db, err := openControlPlaneDB(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO state_blobs (name, payload, updated_at) VALUES ('test', '{}', unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backup", "control-plane.db")
	if err := backupControlPlane(source, backup); err != nil {
		t.Fatal(err)
	}
	if err := verifyControlPlaneBackup(backup); err != nil {
		t.Fatal(err)
	}
}

func TestControlPlaneBackupRejectsMissingSource(t *testing.T) {
	root := t.TempDir()
	if err := backupControlPlane(filepath.Join(root, "missing.db"), filepath.Join(root, "backup.db")); err == nil {
		t.Fatal("backup unexpectedly created a missing source database")
	}
}

func TestControlPlaneRestorePreservesCurrentDatabase(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.db")
	db, err := openControlPlaneDB(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO state_blobs (name, payload, updated_at) VALUES ('source', '{}', unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "live.db")
	db, err = openControlPlaneDB(destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO state_blobs (name, payload, updated_at) VALUES ('old', '{}', unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := restoreControlPlane(source, destination); err != nil {
		t.Fatal(err)
	}
	restored, err := openControlPlaneDB(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var name string
	if err := restored.QueryRow(`SELECT name FROM state_blobs WHERE name = 'source'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination + ".pre-restore-"); err == nil {
		t.Fatal("unexpected fixed backup path")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	foundPrevious := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "live.db.pre-restore-") {
			foundPrevious = true
		}
	}
	if !foundPrevious {
		t.Fatal("current database was not preserved")
	}
}
