package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSiteTransactionRollbackRestoresPreviousSite(t *testing.T) {
	root := t.TempDir()
	recovery := filepath.Join(root, ".stepanel-recovery")
	home := filepath.Join(root, "site", "public")
	writeTestFile(t, filepath.Join(home, "index.html"), "old")
	txn, err := BeginSiteTransaction(recovery, home, "test.restore", "site")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "index.html"), "partial")
	if err := txn.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertTestFile(t, filepath.Join(home, "index.html"), "old")
	assertTestFile(t, filepath.Join(txn.FailedSite, "index.html"), "partial")
	loaded, err := loadSiteTransaction(txn.dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != "rolled-back" {
		t.Fatalf("state = %q, want rolled-back", loaded.State)
	}
}

func TestRecoverSiteTransactionsRollsBackInterruptedSite(t *testing.T) {
	root := t.TempDir()
	recovery := filepath.Join(root, ".stepanel-recovery")
	home := filepath.Join(root, "site", "public")
	writeTestFile(t, filepath.Join(home, "index.html"), "old")
	txn, err := BeginSiteTransaction(recovery, home, "test.restore", "site")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "index.html"), "partial")
	recovered, err := RecoverSiteTransactions(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0] != txn.ID {
		t.Fatalf("recovered = %#v, want %q", recovered, txn.ID)
	}
	assertTestFile(t, filepath.Join(home, "index.html"), "old")
}

func TestSiteTransactionRollbackReconcilesRestoredBackup(t *testing.T) {
	root := t.TempDir()
	recovery := filepath.Join(root, ".stepanel-recovery")
	home := filepath.Join(root, "site", "public")
	writeTestFile(t, filepath.Join(home, "index.html"), "old")
	txn, err := BeginSiteTransaction(recovery, home, "test.restore", "site")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "index.html"), "partial")
	txn.State = "rolling-back"
	if err := txn.persist(); err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(txn.dir, "failed-site")
	if err := os.Rename(home, failed); err != nil {
		t.Fatal(err)
	}
	txn.FailedSite = failed
	if err := txn.persist(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(txn.Backup, home); err != nil {
		t.Fatal(err)
	}
	if err := txn.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertTestFile(t, filepath.Join(home, "index.html"), "old")
	assertTestFile(t, filepath.Join(failed, "index.html"), "partial")
}

func TestCommittedSiteTransactionRetainsBackupUntilCleanup(t *testing.T) {
	root := t.TempDir()
	recovery := filepath.Join(root, ".stepanel-recovery")
	home := filepath.Join(root, "site", "public")
	writeTestFile(t, filepath.Join(home, "index.html"), "old")
	txn, err := BeginSiteTransaction(recovery, home, "test.restore", "site")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "index.html"), "new")
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	assertTestFile(t, filepath.Join(txn.Backup, "index.html"), "old")
	if err := CleanupSiteTransactions(recovery, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(txn.dir); err != nil {
		t.Fatalf("fresh recovery transaction was removed: %v", err)
	}
	if err := CleanupSiteTransactions(recovery, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(txn.dir); !os.IsNotExist(err) {
		t.Fatalf("expired transaction still exists: %v", err)
	}
}

func writeTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0640); err != nil {
		t.Fatal(err)
	}
}

func assertTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
