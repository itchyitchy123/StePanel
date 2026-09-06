package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconcileWorkersRetainsPendingStateWhenHelperFails(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenWorkerStore(filepath.Join(dir, "workers.json"))
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(dir, "helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	store.values["demo/queue"] = Worker{Site: "demo", Name: "queue", Type: "laravel", Processes: 1, MemoryMB: 128, Root: "/var/www/sites/demo/public", State: "pending"}
	app := &App{Config: Config{AppCtl: helper}, Workers: store}
	_, failed := app.reconcileWorkers(context.Background())
	if failed["demo/queue"] == "" {
		t.Fatalf("failed workers = %#v", failed)
	}
	worker := store.values["demo/queue"]
	if worker.State != "pending" || !strings.Contains(worker.LastError, "exit status") {
		t.Fatalf("worker state = %#v", worker)
	}
}
