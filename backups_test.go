package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSiteBackupPublishesVerifiedManifest(t *testing.T) {
	root := t.TempDir()
	webRoot := filepath.Join(root, "www")
	backupRoot := filepath.Join(root, "backups")
	writeTestFile(t, filepath.Join(webRoot, "sites", "account", "public", "index.php"), "<?php echo 'ok';")
	result, err := CreateSiteBackup(Config{WebRoot: webRoot, BackupRoot: backupRoot}, "account", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.ArchiveSHA256 == "" || result.Bytes == 0 || result.VerifiedAt.IsZero() {
		t.Fatalf("incomplete backup result: %#v", result)
	}
	manifest := readTestBackupManifest(t, result.Path)
	if err := VerifyBackupArchive(filepath.Join(result.Path, manifest.Archive), manifest); err != nil {
		t.Fatalf("published backup did not verify: %v", err)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].Path != "site/public/index.php" {
		t.Fatalf("backup entries = %#v", manifest.Entries)
	}
	checksum, err := os.ReadFile(filepath.Join(result.Path, "backup.tar.gz.sha256"))
	if err != nil || string(checksum) != result.ArchiveSHA256+"  backup.tar.gz\n" {
		t.Fatalf("checksum file = %q, error = %v", checksum, err)
	}
}

func TestSignedBackupManifestRequiresValidExternalKey(t *testing.T) {
	root := t.TempDir()
	webRoot := filepath.Join(root, "www")
	backupRoot := filepath.Join(root, "backups")
	writeTestFile(t, filepath.Join(webRoot, "sites", "account", "public", "index.html"), "signed")
	result, err := CreateSiteBackup(Config{WebRoot: webRoot, BackupRoot: backupRoot, BackupSigningKey: "a-secret-key-with-enough-entropy"}, "account", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "manifest.sig")); err != nil {
		t.Fatalf("manifest signature missing: %v", err)
	}
	if _, err := VerifySiteBackup(result.Path, "a-secret-key-with-enough-entropy"); err != nil {
		t.Fatalf("signed backup did not verify: %v", err)
	}
	if _, err := VerifySiteBackup(result.Path, "wrong-key"); err == nil {
		t.Fatal("backup verified with the wrong signing key")
	}
}

func TestBackupManifestReportsLogicalConsistency(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "www", "sites", "account", "public", "index.html"), "logical")
	result, err := CreateSiteBackup(Config{WebRoot: filepath.Join(root, "www"), BackupRoot: filepath.Join(root, "backups")}, "account", false)
	if err != nil {
		t.Fatal(err)
	}
	manifest := readTestBackupManifest(t, result.Path)
	if manifest.Consistency != "crash-consistent / logical backup" || !manifest.ArchiveVerified || manifest.ApplicationQuiesced || manifest.FilesystemSnapshot {
		t.Fatalf("unexpected consistency metadata: %#v", manifest)
	}
}

func TestVerifyBackupArchiveRejectsTampering(t *testing.T) {
	root := t.TempDir()
	webRoot := filepath.Join(root, "www")
	writeTestFile(t, filepath.Join(webRoot, "sites", "account", "public", "index.html"), "original")
	result, err := CreateSiteBackup(Config{WebRoot: webRoot, BackupRoot: filepath.Join(root, "backups")}, "account", false)
	if err != nil {
		t.Fatal(err)
	}
	manifest := readTestBackupManifest(t, result.Path)
	archive := filepath.Join(result.Path, manifest.Archive)
	file, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tampered")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBackupArchive(archive, manifest); err == nil {
		t.Fatal("tampered backup passed verification")
	}
}

func TestCreateSiteBackupIncludesManagedDatabaseDump(t *testing.T) {
	root := t.TempDir()
	webRoot := filepath.Join(root, "www")
	writeTestFile(t, filepath.Join(webRoot, "sites", "account", "public", "index.html"), "site")
	helper := filepath.Join(root, "dbctl")
	script := "#!/bin/sh\ncase \"$1\" in\n  list) printf 'account_blog\\tcpmove:account\\n';;\n  dump) printf '%s\\n' 'CREATE TABLE posts (id INT);';;\n  *) exit 1;;\nesac\n"
	if err := os.WriteFile(helper, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := CreateSiteBackup(Config{WebRoot: webRoot, BackupRoot: filepath.Join(root, "backups"), DBCtl: helper}, "account", true)
	if err != nil {
		t.Fatal(err)
	}
	manifest := readTestBackupManifest(t, result.Path)
	if len(manifest.Databases) != 1 || manifest.Databases[0] != "account_blog" {
		t.Fatalf("managed databases = %#v", manifest.Databases)
	}
	found := false
	for _, entry := range manifest.Entries {
		if entry.Path == "databases/account_blog.sql" {
			found = true
		}
	}
	if !found {
		t.Fatalf("database dump is absent from manifest: %#v", manifest.Entries)
	}
}

func readTestBackupManifest(t *testing.T, root string) BackupManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
