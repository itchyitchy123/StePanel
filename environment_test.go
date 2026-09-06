package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveEnvironmentRestoresStateAfterPersistenceFailure(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(dir, "helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ncat >/dev/null\n"), 0700); err != nil {
		t.Fatal(err)
	}
	store := &EnvironmentStore{
		path: filepath.Join(blocked, "environments.json"),
		values: map[string]map[string]environmentValue{
			"demo": {"APP_ENV": {Value: "production"}},
		},
	}
	app := &App{Config: Config{AppCtl: helper}, Environments: store}
	if err := app.removeEnvironment(context.Background(), "demo"); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := store.values["demo"]["APP_ENV"].Value; got != "production" {
		t.Fatalf("environment state after failed removal = %q", got)
	}
}
