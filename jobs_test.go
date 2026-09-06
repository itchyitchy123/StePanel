package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestJobsIdempotentRestoreReturnsExistingJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	jobs, err := OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	work := func() (ImportResult, error) {
		calls.Add(1)
		close(started)
		<-release
		return ImportResult{User: "site"}, nil
	}
	first, existing, err := jobs.SubmitIdempotent("restore-1", "site", "deploy-123", work)
	if err != nil || existing || first != "restore-1" {
		t.Fatalf("first submit = id %q existing %v err %v", first, existing, err)
	}
	<-started
	second, existing, err := jobs.SubmitIdempotent("restore-2", "site", "deploy-123", work)
	if err != nil || !existing || second != first {
		t.Fatalf("retry submit = id %q existing %v err %v; want original job", second, existing, err)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := jobs.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("restore work calls = %d, want 1", got)
	}
	reopened, err := OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	retryID, existing, err := reopened.SubmitIdempotent("restore-3", "site", "deploy-123", func() (ImportResult, error) {
		t.Fatal("persisted idempotency key launched duplicate work")
		return ImportResult{}, nil
	})
	if err != nil || !existing || retryID != first {
		t.Fatalf("post-restart retry = id %q existing %v err %v; want original job", retryID, existing, err)
	}
}

func TestValidJobOperationKey(t *testing.T) {
	for _, value := range []string{"retry-1", "github.delivery:abc_123", "a"} {
		if !validJobOperationKey(value) {
			t.Errorf("valid operation key %q was rejected", value)
		}
	}
	for _, value := range []string{"", "with space", "with/slash", strings.Repeat("a", 129)} {
		if validJobOperationKey(value) {
			t.Errorf("invalid operation key %q was accepted", value)
		}
	}
}

func TestJobsPersistCompletedBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	jobs, err := OpenJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SubmitBackup("backup-1", "site", func() (BackupResult, error) {
		return BackupResult{Site: "site", Path: "/backups/site", ArchiveSHA256: strings.Repeat("a", 64)}, nil
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
	job, ok := reopened.Get("backup-1")
	if !ok || job.State != "completed" || job.Backup == nil || job.Backup.Site != "site" {
		t.Fatalf("persisted backup job = %#v, found = %v", job, ok)
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
