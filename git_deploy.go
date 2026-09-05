package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var gitRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@+-]{0,127}$`)
var gitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type gitDeployRequest struct {
	Site       string `json:"site"`
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
}

type gitDeployResult struct {
	Site       string `json:"site"`
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit"`
	Previous   string `json:"previous_release,omitempty"`
}

// gitDeploy intentionally does not evaluate repository-provided build scripts.
// Build execution belongs in a separately sandboxed runner.
func (a *App) gitDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	var input gitDeployRequest
	if err := decodeJSON(w, r, 16<<10, &input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	input.Site = safeUser(input.Site)
	input.Repository = strings.TrimSpace(input.Repository)
	input.Ref = strings.TrimSpace(input.Ref)
	if input.Ref == "" {
		input.Ref = "main"
	}
	if input.Site == "" || !gitRefPattern.MatchString(input.Ref) {
		http.Error(w, "invalid site or Git ref", http.StatusUnprocessableEntity)
		return
	}
	repository, err := url.Parse(input.Repository)
	if err != nil || repository.Scheme != "https" || repository.User != nil || repository.Host == "" || repository.RawQuery != "" || repository.Fragment != "" || !strings.HasSuffix(strings.ToLower(repository.Path), ".git") {
		http.Error(w, "repository must be an HTTPS .git URL without credentials or query parameters", http.StatusUnprocessableEntity)
		return
	}
	publicRoot := filepath.Join(a.Config.WebRoot, "sites", input.Site, "public")
	if err := ensureInside(a.Config.WebRoot, publicRoot); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	siteRoot := filepath.Dir(publicRoot)
	if info, err := os.Stat(siteRoot); err != nil || !info.IsDir() {
		http.Error(w, "site root does not exist", http.StatusUnprocessableEntity)
		return
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		http.Error(w, "Git is not installed", http.StatusServiceUnavailable)
		return
	}
	release := filepath.Join(siteRoot, ".stepanel-release-"+strings.ReplaceAll(newRequestID(), "-", ""))
	if err := os.Mkdir(release, 0700); err != nil {
		http.Error(w, "unable to create release staging directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(release)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	if output, err := runBoundedCommand(ctx, exec.CommandContext(ctx, gitPath, "clone", "--depth", "1", "--branch", input.Ref, "--single-branch", input.Repository, release)); err != nil {
		http.Error(w, "Git checkout failed: "+strings.TrimSpace(string(output)), http.StatusBadGateway)
		return
	}
	commitOutput, err := runBoundedCommand(ctx, exec.CommandContext(ctx, gitPath, "-C", release, "rev-parse", "HEAD"))
	if err != nil {
		http.Error(w, "unable to identify checked-out commit", http.StatusBadGateway)
		return
	}
	commit := strings.TrimSpace(string(commitOutput))
	if !gitCommitPattern.MatchString(commit) {
		http.Error(w, "Git returned an invalid commit identifier", http.StatusBadGateway)
		return
	}
	previous := ""
	if _, err := os.Stat(publicRoot); err == nil {
		previous = filepath.Join(siteRoot, ".stepanel-previous-"+time.Now().UTC().Format("20060102-150405"))
		if err := os.Rename(publicRoot, previous); err != nil {
			http.Error(w, "unable to preserve the current release", http.StatusInternalServerError)
			return
		}
	}
	if err := os.Rename(release, publicRoot); err != nil {
		if previous != "" {
			_ = os.Rename(previous, publicRoot)
		}
		http.Error(w, "unable to activate the new release", http.StatusInternalServerError)
		return
	}
	if err := siteHelper(a.Config, "seal", input.Site); err != nil {
		_ = os.RemoveAll(publicRoot)
		if previous != "" {
			_ = os.Rename(previous, publicRoot)
		}
		http.Error(w, "site isolation could not be restored", http.StatusServiceUnavailable)
		return
	}
	result := gitDeployResult{Site: input.Site, Repository: input.Repository, Ref: input.Ref, Commit: commit, Previous: previous}
	_ = AuditAs(a.Config.AuditLog, a.Auth.Username, "site.git-deployed", input.Site, input.Repository+"@"+commit)
	writeJSON(w, http.StatusAccepted, result)
}

func newRequestID() string {
	id, err := randomSecret()
	if err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return id
}
