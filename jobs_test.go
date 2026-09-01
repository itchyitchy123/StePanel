package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJobsPersistCompletedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	jobs, err := OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Submit("restore-1", "site", func() (ImportResult, error) {
		return ImportResult{User: "site", FilesRestored: true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := jobs.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	job, ok := reopened.Get("restore-1")
	if !ok || job.State != "completed" || job.Result == nil || !job.Result.FilesRestored {
		t.Fatalf("persisted job = %#v, found = %v", job, ok)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("job state mode = %o, want 600", info.Mode().Perm())
	}
}

func TestOpenJobsReconcilesInterruptedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	store := jobStore{Version: 1, Jobs: []*Job{{ID: "restore-1", Kind: "cpmove.restore", State: "running", User: "site", StartedAt: time.Now().Add(-time.Minute)}}}
	data, err := json.Marshal(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	jobs, err := OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	job, ok := jobs.Get("restore-1")
	if !ok || job.State != "failed" || job.FinishedAt == nil || !strings.Contains(job.Error, "unclean shutdown") {
		t.Fatalf("reconciled job = %#v, found = %v", job, ok)
	}
}

func TestOpenJobsRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJobs(path); err == nil {
		t.Fatal("corrupt job state was accepted")
	}
}

func TestJobsFailClosedWhenStateCannotBePersisted(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "state")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	jobs, err := OpenJobs(filepath.Join(blocked, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	err = jobs.Submit("restore-1", "site", func() (ImportResult, error) {
		called = true
		return ImportResult{}, nil
	})
	if err == nil || called {
		t.Fatalf("submit error = %v, work called = %v", err, called)
	}
	if _, ok := jobs.Get("restore-1"); ok {
		t.Fatal("unpersisted job remained visible")
	}
}
