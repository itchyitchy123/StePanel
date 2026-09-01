package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupImportStagesRemovesOnlyExpiredRestoreStages(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "20200101-010101-account")
	keep := filepath.Join(root, "20990101-010101-account")
	nonStage := filepath.Join(root, "notes")
	orphanUpload := filepath.Join(root, "upload-old.tar.gz")
	orphanWPress := filepath.Join(root, "wpress-upload-old.wpress")
	for _, path := range []string{old, keep, nonStage} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{orphanUpload, orphanWPress} {
		if err := os.WriteFile(path, []byte("stale"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(old, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := CleanupImportStages(root, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old stage still exists: %v", err)
	}
	for _, path := range []string{orphanUpload, orphanWPress} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("orphan upload still exists: %s: %v", path, err)
		}
	}
	for _, path := range []string{keep, nonStage} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unexpected removal of %s: %v", path, err)
		}
	}
}
