package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var wpressNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)
var wpressPrefixPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)

type WPressResult struct {
	Site             string `json:"site"`
	Home             string `json:"home"`
	Database         string `json:"database"`
	DatabaseUser     string `json:"database_user"`
	SourcePrefix     string `json:"source_prefix"`
	TargetPrefix     string `json:"target_prefix"`
	FilesRestored    bool   `json:"files_restored"`
	DatabaseRestored bool   `json:"database_restored"`
	URLReplaced      bool   `json:"url_replaced"`
	StagedAt         string `json:"staged_at"`
}

func WPressPreflight(cfg Config) map[string]bool {
	return map[string]bool{
		"wpress_extract":  commandAvailable(cfg.WPressExtract),
		"wp_cli":          commandAvailable(cfg.WPCLI),
		"database_client": commandAvailable("mariadb") || commandAvailable("mysql"),
	}
}

func commandAvailable(name string) bool {
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func (a *App) wpressPreflight(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ready": allReady(WPressPreflight(a.Config)), "checks": WPressPreflight(a.Config)})
}

func allReady(checks map[string]bool) bool {
	for _, ready := range checks {
		if !ready {
			return false
		}
	}
	return true
}

func validateWPressInput(site, dbSuffix, dbUserSuffix, password, targetPrefix, siteURL string) error {
	if safeUser(site) == "" {
		return errors.New("invalid site account")
	}
	if !wpressNamePattern.MatchString(dbSuffix) || !wpressNamePattern.MatchString(dbUserSuffix) {
		return errors.New("database names may contain only letters, numbers, and underscores")
	}
	if !wpressPrefixPattern.MatchString(targetPrefix) {
		return errors.New("invalid WordPress table prefix")
	}
	if len(password) < 12 || strings.ContainsAny(password, "\r\n") {
		return errors.New("database password must be at least 12 characters and contain no newlines")
	}
	if siteURL != "" {
		u, err := url.Parse(siteURL)
		if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
			return errors.New("site URL must be an http or https URL")
		}
	}
	return nil
}

func (a *App) wpressImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.Auth.CSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if !allReady(WPressPreflight(a.Config)) {
		http.Error(w, "WPress dependencies are not installed; check /api/wpress/preflight", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if r.FormValue("confirm") != "WPRESTORE" {
		http.Error(w, "type WPRESTORE to authorize the restore", http.StatusBadRequest)
		return
	}
	site := safeUser(r.FormValue("site"))
	dbSuffix := strings.TrimSpace(r.FormValue("db_name"))
	dbUserSuffix := strings.TrimSpace(r.FormValue("db_user"))
	password := r.FormValue("db_password")
	targetPrefix := strings.TrimSpace(r.FormValue("table_prefix"))
	if targetPrefix == "" {
		targetPrefix = "wp_"
	}
	siteURL := strings.TrimRight(strings.TrimSpace(r.FormValue("site_url")), "/")
	if err := validateWPressInput(site, dbSuffix, dbUserSuffix, password, targetPrefix, siteURL); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	file, header, err := r.FormFile("backup")
	if err != nil || !strings.EqualFold(filepath.Ext(header.Filename), ".wpress") {
		http.Error(w, "a .wpress archive is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	temp, err := os.CreateTemp(a.Config.ImportRoot, "wpress-upload-*.wpress")
	if err != nil {
		http.Error(w, "could not stage upload", http.StatusInternalServerError)
		return
	}
	tempPath := temp.Name()
	if _, err = io.Copy(temp, file); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		http.Error(w, "could not stage upload", http.StatusInternalServerError)
		return
	}
	if err = temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		http.Error(w, "could not stage upload", http.StatusInternalServerError)
		return
	}
	force := r.FormValue("overwrite") == "on"
	jobID := time.Now().UTC().Format("20060102-150405.000000000") + "-" + site + "-wpress"
	if !a.Jobs.SubmitWPress(jobID, site, func() (WPressResult, error) {
		defer os.Remove(tempPath)
		result, restoreErr := RestoreWPress(a.Config, tempPath, site, dbSuffix, dbUserSuffix, password, siteURL, targetPrefix, force)
		if restoreErr != nil {
			_ = Audit(a.Config.AuditLog, "wordpress.restore.failed", site, restoreErr.Error())
		} else {
			_ = Audit(a.Config.AuditLog, "wordpress.restore.completed", site, result.StagedAt)
		}
		return result, restoreErr
	}) {
		_ = os.Remove(tempPath)
		http.Error(w, "too many long-running jobs", http.StatusTooManyRequests)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status_url": filepath.Join("/api/jobs", jobID)})
}

func RestoreWPress(cfg Config, archive, site, dbSuffix, dbUserSuffix, dbPassword, siteURL, targetPrefix string, force bool) (WPressResult, error) {
	if err := validateWPressInput(site, dbSuffix, dbUserSuffix, dbPassword, targetPrefix, siteURL); err != nil {
		return WPressResult{}, err
	}
	if !commandAvailable(cfg.WPressExtract) || !commandAvailable(cfg.WPCLI) {
		return WPressResult{}, errors.New("wpress-extract and wp-cli are required")
	}
	dbName := site + "_" + dbSuffix
	dbUser := site + "_" + dbUserSuffix
	if len(dbName) > 64 || len(dbUser) > 32 {
		return WPressResult{}, errors.New("database name or user is too long")
	}
	stage, err := os.MkdirTemp(cfg.ImportRoot, "wpress-restore-")
	if err != nil {
		return WPressResult{}, err
	}
	defer os.RemoveAll(stage)
	extracted := filepath.Join(stage, "extracted")
	if err := runCommand(20*time.Minute, cfg.WPressExtract, "--out", extracted, archive); err != nil {
		return WPressResult{}, fmt.Errorf("extract archive: %w", err)
	}
	source := findWordPressRoot(extracted)
	if source == "" || !fileExists(filepath.Join(source, "database.sql")) {
		return WPressResult{}, errors.New("archive must contain WordPress files and database.sql")
	}
	home := filepath.Join(cfg.WebRoot, "sites", site, "public")
	if err := ensureInside(cfg.WebRoot, home); err != nil {
		return WPressResult{}, err
	}
	if existing, statErr := os.Lstat(home); statErr == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			return WPressResult{}, errors.New("destination site is a symlink")
		}
		if !force {
			return WPressResult{}, errors.New("destination is not empty; enable overwrite only after taking a backup")
		}
		if err := os.Rename(home, filepath.Join(stage, "site-before")); err != nil {
			return WPressResult{}, fmt.Errorf("snapshot existing site: %w", err)
		}
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(home)
			_ = os.Rename(filepath.Join(stage, "site-before"), home)
		}
	}()
	if err := os.MkdirAll(home, 0750); err != nil {
		return WPressResult{}, err
	}
	if err := copyWPressTree(source, home); err != nil {
		return WPressResult{}, fmt.Errorf("restore WordPress files: %w", err)
	}
	if err := createWPressDatabase(cfg, dbName, dbUser, dbPassword); err != nil {
		return WPressResult{}, err
	}
	if err := importWPressDatabase(cfg, dbName, filepath.Join(source, "database.sql")); err != nil {
		return WPressResult{}, err
	}
	if err := configureWordPress(cfg, home, dbName, dbUser, dbPassword, targetPrefix); err != nil {
		return WPressResult{}, err
	}
	sourcePrefix, err := detectWPressPrefix(cfg, dbName)
	if err != nil {
		return WPressResult{}, err
	}
	if err := runWP(cfg, home, "config", "set", "table_prefix", sourcePrefix, "--type=variable"); err != nil {
		return WPressResult{}, err
	}
	if err := runWP(cfg, home, "search-replace", sourcePrefix, targetPrefix, "--all-tables", "--precise", "--recurse-objects", "--quiet"); err != nil {
		return WPressResult{}, fmt.Errorf("normalize table prefix: %w", err)
	}
	if err := runWP(cfg, home, "config", "set", "table_prefix", targetPrefix, "--type=variable"); err != nil {
		return WPressResult{}, err
	}
	urlReplaced := false
	if siteURL != "" {
		oldURL, getErr := runWPOutput(cfg, home, "option", "get", "siteurl")
		if getErr == nil && strings.TrimSpace(oldURL) != "" && strings.TrimSpace(oldURL) != siteURL {
			if err := runWP(cfg, home, "search-replace", strings.TrimSpace(oldURL), siteURL, "--all-tables-with-prefix", "--precise", "--recurse-objects", "--skip-columns=guid", "--quiet"); err != nil {
				return WPressResult{}, fmt.Errorf("replace site URL: %w", err)
			}
			_ = runWP(cfg, home, "option", "update", "home", siteURL)
			_ = runWP(cfg, home, "option", "update", "siteurl", siteURL)
			urlReplaced = true
		}
	}
	_ = runWP(cfg, home, "rewrite", "flush")
	_ = runWP(cfg, home, "cache", "flush")
	committed = true
	return WPressResult{Site: site, Home: home, Database: dbName, DatabaseUser: dbUser, SourcePrefix: sourcePrefix, TargetPrefix: targetPrefix, FilesRestored: true, DatabaseRestored: true, URLReplaced: urlReplaced, StagedAt: stage}, nil
}

func findWordPressRoot(root string) string {
	if fileExists(filepath.Join(root, "database.sql")) {
		return root
	}
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" || !info.IsDir() {
			return nil
		}
		if fileExists(filepath.Join(path, "database.sql")) && (fileExists(filepath.Join(path, "wp-config.php")) || fileExists(filepath.Join(path, "wp-config-sample.php"))) {
			found = path
		}
		return nil
	})
	return found
}

func copyWPressTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == filepath.Join(src, "database.sql") {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		return copyFile(path, target, 0640)
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

func runCommand(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runWP(cfg Config, home string, args ...string) error {
	base := []string{"--path=" + home, "--skip-plugins", "--skip-themes"}
	base = append(base, args...)
	return runCommand(10*time.Minute, cfg.WPCLI, base...)
}

func runWPOutput(cfg Config, home string, args ...string) (string, error) {
	base := []string{"--path=" + home, "--skip-plugins", "--skip-themes"}
	base = append(base, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, cfg.WPCLI, base...).CombinedOutput()
	return string(output), err
}

func configureWordPress(cfg Config, home, dbName, dbUser, dbPassword, prefix string) error {
	if !fileExists(filepath.Join(home, "wp-config.php")) {
		if err := runWP(cfg, home, "config", "create", "--dbname="+dbName, "--dbuser="+dbUser, "--dbpass="+dbPassword, "--dbhost="+cfg.DBHost, "--dbprefix="+prefix, "--skip-check", "--skip-salts"); err != nil {
			return fmt.Errorf("create wp-config.php: %w", err)
		}
	}
	for _, setting := range [][2]string{{"DB_NAME", dbName}, {"DB_USER", dbUser}, {"DB_PASSWORD", dbPassword}, {"DB_HOST", cfg.DBHost}} {
		if err := runWP(cfg, home, "config", "set", setting[0], setting[1], "--type=constant"); err != nil {
			return fmt.Errorf("configure %s: %w", setting[0], err)
		}
	}
	return nil
}

func detectWPressPrefix(cfg Config, dbName string) (string, error) {
	for _, candidate := range []string{"SERVMASK_PREFIX", "SRVMASK", "SERVMASK", "wp_"} {
		query := "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=" + sqlString(dbName) + " AND TABLE_NAME LIKE " + sqlString(candidate+"%") + " LIMIT 1"
		output, err := runMySQL(cfg, query)
		if err == nil && strings.TrimSpace(output) != "" {
			return candidate, nil
		}
	}
	return "", errors.New("could not detect the WordPress table prefix")
}

func createWPressDatabase(cfg Config, dbName, dbUser, password string) error {
	query := "CREATE DATABASE IF NOT EXISTS " + sqlIdent(dbName) + "; CREATE USER IF NOT EXISTS " + sqlString(dbUser) + "@'localhost' IDENTIFIED BY " + sqlString(password) + "; GRANT ALL PRIVILEGES ON " + sqlIdent(dbName) + ".* TO " + sqlString(dbUser) + "@'localhost'; FLUSH PRIVILEGES;"
	if _, err := runMySQL(cfg, query); err != nil {
		return fmt.Errorf("provision WordPress database: %w", err)
	}
	return nil
}

func importWPressDatabase(cfg Config, dbName, dump string) error {
	input, err := os.Open(dump)
	if err != nil {
		return err
	}
	defer input.Close()
	args := append(mysqlArgs(cfg), dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, mysqlClient(), args...)
	if cfg.DBPassword != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
	}
	cmd.Stdin = input
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("import database: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runMySQL(cfg Config, query string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	args := append(mysqlArgs(cfg), "--batch", "--skip-column-names", "--execute", query)
	cmd := exec.CommandContext(ctx, mysqlClient(), args...)
	if cfg.DBPassword != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func mysqlClient() string {
	if path, err := exec.LookPath("mariadb"); err == nil {
		return path
	}
	return "mysql"
}

func sqlString(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
func sqlIdent(value string) string  { return "`" + strings.ReplaceAll(value, "`", "``") + "`" }
