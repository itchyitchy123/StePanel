package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitAllowedHostsRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "localhost", "127.0.0.1", "github.com:443", "github.com/evil"} {
		if err := validateGitAllowedHosts(value); err == nil {
			t.Fatalf("allowed unsafe Git host list %q", value)
		}
	}
	if err := validateGitAllowedHosts("github.com,git.example.net"); err != nil {
		t.Fatalf("rejected valid Git hosts: %v", err)
	}
	if !gitHostAllowed("github.com,git.example.net", "GIT.EXAMPLE.NET") || gitHostAllowed("github.com", "evil.github.com") {
		t.Fatal("Git host allowlist did not use exact case-insensitive matching")
	}
}

func TestValidateGitReleaseRejectsSymlinksAndEntryFloods(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateGitRelease(root, 2); err != nil {
		t.Fatalf("safe release rejected: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "secret")); err != nil {
		t.Fatal(err)
	}
	if err := validateGitRelease(root, 10); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink release result = %v", err)
	}
	if err := validateGitRelease(root, 1); err == nil || !strings.Contains(err.Error(), "entry limit") {
		t.Fatalf("entry-limit result = %v", err)
	}
}
