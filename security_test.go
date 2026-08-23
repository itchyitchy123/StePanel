package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityChecksDetectPrivateImportRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	app := &App{Config: Config{ImportRoot: root, WebRoot: filepath.Join(root, "public"), Production: false}, Auth: Auth{Enabled: true}}
	checks := app.SecurityChecks()
	if checks[0].Status != "pass" {
		t.Fatalf("authentication check = %q, want pass", checks[0].Status)
	}
	if checks[1].Status != "pass" {
		t.Fatalf("import permissions check = %q, want pass", checks[1].Status)
	}
}

func TestFormatUptime(t *testing.T) {
	if got := formatUptime(90061); got != "1d 01h" {
		t.Fatalf("formatUptime() = %q, want 1d 01h", got)
	}
}
