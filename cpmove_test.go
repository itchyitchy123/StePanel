package main

import (
	"os"
	"path/filepath"
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
