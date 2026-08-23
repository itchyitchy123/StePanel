package main

import (
	"archive/tar"
	"compress/gzip"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeArchivePath(t *testing.T) {
	valid := []string{"homedir/public_html/index.php", "mysql/site.sql", "homedir/.well-known/acme-challenge/token"}
	for _, path := range valid {
		if !safeArchivePath(path) {
			t.Errorf("expected archive path to be valid: %q", path)
		}
	}
	invalid := []string{"/etc/passwd", "../../etc/passwd", "homedir/../../etc/passwd", "..", `homedir\\..\\etc\\passwd`}
	for _, path := range invalid {
		if safeArchivePath(path) {
			t.Errorf("expected archive path to be rejected: %q", path)
		}
	}
}

func TestSafeUser(t *testing.T) {
	for _, value := range []string{"stephen", "site_01", "site-name"} {
		if safeUser(value) != value {
			t.Errorf("expected username %q to be accepted", value)
		}
	}
	for _, value := range []string{"Stephen", "site name", "site/../../etc", ""} {
		if safeUser(value) != "" {
			t.Errorf("expected username %q to be rejected", value)
		}
	}
}

func TestRestoreMailStagesMailboxData(t *testing.T) {
	stage := t.TempDir()
	mailRoot := t.TempDir()
	source := filepath.Join(stage, "homedir", "mail", "example.com", "alice", "cur")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "message"), []byte("mail"), 0600); err != nil {
		t.Fatal(err)
	}
	staged, mailboxes, failures := restoreMail(Config{MailRoot: mailRoot}, stage, "account")
	if !staged || len(failures) != 0 {
		t.Fatalf("restoreMail() = staged %v, failures %v", staged, failures)
	}
	if len(mailboxes) != 1 || mailboxes[0] != "example.com/alice" {
		t.Fatalf("mailboxes = %v, want [example.com/alice]", mailboxes)
	}
	if _, err := os.Stat(filepath.Join(mailRoot, "account", "mail", "example.com", "alice", "cur", "message")); err != nil {
		t.Fatalf("staged message missing: %v", err)
	}
}

func TestRestoreFailureRestoresExistingSite(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "sites", "account", "public")
	if err := os.MkdirAll(home, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "marker.txt"), []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "backup.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gz)
	body := []byte("<?php system($_GET['cmd']);")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "homedir/public_html/shell.php", Mode: 0600, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}
	_, restoreErr := RestoreCPMove(Config{ImportRoot: filepath.Join(root, "imports"), WebRoot: root, MailRoot: filepath.Join(root, "mail")}, input, &multipart.FileHeader{Filename: "cpmove-account.tar.gz", Size: info.Size()}, "account", false)
	if restoreErr == nil || !strings.Contains(restoreErr.Error(), "restore blocked") {
		t.Fatalf("restore error = %v", restoreErr)
	}
	marker, err := os.ReadFile(filepath.Join(home, "marker.txt"))
	if err != nil || string(marker) != "keep me" {
		t.Fatalf("existing site was not restored: %q, %v", marker, err)
	}
}
