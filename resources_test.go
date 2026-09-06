package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenResourceStoreNormalizesLegacyProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.json")
	data := `{"demo":{"cpu_percent":100,"memory_mb":512,"tasks_max":64,"php_workers":4,"state":"applied"}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenResourceStore(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := store.values["demo"]
	if profile.Site != "demo" || profile.CPUWeight != 100 || profile.MemoryHighMB != 460 || profile.IOWeight != 100 {
		t.Fatalf("normalized profile = %#v", profile)
	}
}

func TestOpenResourceStoreRejectsInvalidPersistedProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.json")
	data := `{"demo":{"site":"demo","cpu_percent":1,"memory_mb":512,"tasks_max":64,"php_workers":4,"state":"applied"}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenResourceStore(path); err == nil || !strings.Contains(err.Error(), "invalid resource profile") {
		t.Fatalf("expected invalid profile error, got %v", err)
	}
}

func TestEnsurePlanResourcesPersistsEnforcedEnvelopeAsPendingOnHelperFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.json")
	store, err := OpenResourceStore(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Resources: store}
	pending, err := app.ensurePlanResources(HostingAccount{Plan: "starter", Sites: []string{"demo"}})
	if err != nil || len(pending) != 1 || pending[0] != "demo" {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	profile, ok := store.values["demo"]
	if !ok || profile.State != "pending" || profile.MemoryMB != 512 || profile.CPUPercent != 100 || profile.TasksMax != 128 || profile.PHPWorkers != 8 {
		t.Fatalf("plan profile = %#v", profile)
	}
}
