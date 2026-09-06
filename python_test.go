package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcilePythonAppsRetainsPendingStateWhenHelperFails(t *testing.T) {
	dir := t.TempDir()
	appRoot := filepath.Join(dir, "apps")
	app := PythonApp{Site: "demo", Version: "3.13", EntryPoint: "app:app", Port: 8000, Workers: 2, Root: "/var/www/sites/demo/public", State: "pending"}
	data, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appRoot, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pythonManifestPath(appRoot, app.Site), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(dir, "helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	service := &App{Config: Config{AppRoot: appRoot, AppCtl: helper}}
	_, failed := service.reconcilePythonApps(context.Background())
	if failed[app.Site] == "" {
		t.Fatalf("failed applications = %#v", failed)
	}
	updated, err := os.ReadFile(pythonManifestPath(appRoot, app.Site))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), `"state": "pending"`) || !strings.Contains(string(updated), "exit status") {
		t.Fatalf("pending Python manifest = %q", updated)
	}
}
