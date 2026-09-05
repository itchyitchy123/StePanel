package main

import (
	"net/http"
	"net/http/httptest"
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

func TestGitRollbackAtomicallySwapsPreviousRelease(t *testing.T) {
	webRoot := filepath.Join(t.TempDir(), "www")
	siteRoot := filepath.Join(webRoot, "sites", "example")
	publicRoot := filepath.Join(siteRoot, "public")
	previousRoot := filepath.Join(siteRoot, ".stepanel-previous-old")
	if err := os.MkdirAll(publicRoot, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(previousRoot, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicRoot, "version"), []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previousRoot, "version"), []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{Config: Config{WebRoot: webRoot, MaxEntries: 100}}
	request := httptest.NewRequest(http.MethodPost, "/api/sites/git-rollback", strings.NewReader(`{"site":"example","confirm":"ROLLBACK example"}`))
	response := httptest.NewRecorder()
	app.gitRollback(response, request)
	data, err := os.ReadFile(filepath.Join(publicRoot, "version"))
	if response.Code != http.StatusOK || err != nil || string(data) != "previous" {
		t.Fatalf("status = %d, active = %q, error = %v, body = %s", response.Code, data, err, response.Body.String())
	}
	entries, err := os.ReadDir(siteRoot)
	if err != nil {
		t.Fatal(err)
	}
	foundCurrent := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".stepanel-previous-") {
			body, readErr := os.ReadFile(filepath.Join(siteRoot, entry.Name(), "version"))
			foundCurrent = foundCurrent || readErr == nil && string(body) == "current"
		}
	}
	if !foundCurrent {
		t.Fatal("rollback did not preserve the replaced active release")
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
