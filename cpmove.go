package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CPMoveInfo struct {
	Archive   string   `json:"archive"`
	Entries   int      `json:"entries"`
	User      string   `json:"detected_user"`
	HasHome   bool     `json:"has_home"`
	HasMySQL  bool     `json:"has_mysql"`
	HasMail   bool     `json:"has_mail"`
	Mailboxes []string `json:"mailboxes,omitempty"`
	Databases []string `json:"databases"`
}
type ImportResult struct {
	User              string   `json:"user"`
	Home              string   `json:"home"`
	FilesRestored     bool     `json:"files_restored"`
	DatabasesRestored []string `json:"databases_restored"`
	DatabaseErrors    []string `json:"database_errors,omitempty"`
	MailStaged        bool     `json:"mail_staged"`
	MailboxesStaged   []string `json:"mailboxes_staged,omitempty"`
	MailErrors        []string `json:"mail_errors,omitempty"`
	StagedAt          string   `json:"staged_at"`
}

func InspectCPMove(file multipart.File, header *multipart.FileHeader) (CPMoveInfo, error) {
	return inspectCPMove(file, header, 1000000)
}

func inspectCPMove(file multipart.File, header *multipart.FileHeader, maxEntries int) (CPMoveInfo, error) {
	if header.Size > 20<<30 {
		return CPMoveInfo{}, errors.New("backup exceeds the 20 GiB upload limit")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return CPMoveInfo{}, err
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return CPMoveInfo{}, errors.New("backup is not a valid gzip archive")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	info := CPMoveInfo{Archive: header.Filename}
	seen := map[string]bool{}
	seenMail := map[string]bool{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return info, errors.New("backup tar stream is damaged")
		}
		if !safeArchivePath(h.Name) {
			return info, fmt.Errorf("unsafe archive path: %s", h.Name)
		}
		info.Entries++
		if maxEntries > 0 && info.Entries > maxEntries {
			return info, errors.New("archive contains too many entries")
		}
		name := strings.Trim(h.Name, "/")
		parts := strings.Split(name, "/")
		if len(parts) > 1 && parts[0] == "homedir" {
			info.HasHome = true
		}
		if strings.HasPrefix(name, "mysql/") || strings.Contains(name, "/mysql/") {
			info.HasMySQL = true
			if strings.HasSuffix(name, ".sql") {
				db := strings.TrimSuffix(filepath.Base(name), ".sql")
				if !seen[db] {
					info.Databases = append(info.Databases, db)
					seen[db] = true
				}
			}
		}
		if strings.HasPrefix(name, "homedir/mail/") {
			info.HasMail = true
			if len(parts) >= 4 {
				mailbox := parts[2] + "/" + parts[3]
				if !seenMail[mailbox] {
					info.Mailboxes = append(info.Mailboxes, mailbox)
					seenMail[mailbox] = true
				}
			}
		}
		if info.User == "" && strings.HasPrefix(name, "homedir/") && len(parts) > 1 {
			info.User = parts[1]
		}
	}
	sort.Strings(info.Databases)
	sort.Strings(info.Mailboxes)
	if info.Entries == 0 {
		return info, errors.New("backup archive is empty")
	}
	return info, nil
}

func RestoreCPMove(cfg Config, file multipart.File, header *multipart.FileHeader, user string, databases bool) (ImportResult, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ImportResult{}, err
	}
	if _, err := inspectCPMove(file, header, cfg.MaxEntries); err != nil {
		return ImportResult{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ImportResult{}, err
	}
	id := time.Now().UTC().Format("20060102-150405") + "-" + user
	stage := filepath.Join(cfg.ImportRoot, id)
	if err := os.MkdirAll(stage, 0700); err != nil {
		return ImportResult{}, err
	}
	archive := filepath.Join(stage, "backup.tar.gz")
	out, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ImportResult{}, err
	}
	if _, err = io.Copy(out, file); err != nil {
		out.Close()
		return ImportResult{}, err
	}
	out.Close()
	if err = extractArchive(archive, stage); err != nil {
		return ImportResult{}, err
	}
	home := filepath.Join(cfg.WebRoot, "sites", user, "public")
	backup := filepath.Join(stage, "site-before")
	if existing, statErr := os.Lstat(home); statErr == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			return ImportResult{}, errors.New("destination site is a symlink")
		}
		if err = os.Rename(home, backup); err != nil {
			return ImportResult{}, fmt.Errorf("snapshot existing site: %w", err)
		}
	}
	if err = os.MkdirAll(home, 0750); err != nil {
		_ = os.RemoveAll(home)
		_ = os.Rename(backup, home)
		return ImportResult{}, err
	}
	source := firstExisting(filepath.Join(stage, "homedir", "public_html"), filepath.Join(stage, "homedir", user, "public_html"))
	if source != "" {
		if findings, scanErr := scanPHP(source); scanErr != nil {
			return ImportResult{}, fmt.Errorf("malware scan failed: %w", scanErr)
		} else if len(findings) > 0 {
			return ImportResult{}, fmt.Errorf("restore blocked: malware scan detected %d suspicious PHP file(s)", len(findings))
		}
		if err = copyTree(source, home); err != nil {
			_ = os.RemoveAll(home)
			_ = os.Rename(backup, home)
			return ImportResult{}, err
		}
	}
	result := ImportResult{User: user, Home: home, FilesRestored: firstExisting(filepath.Join(stage, "homedir", "public_html"), filepath.Join(stage, "homedir", user, "public_html")) != "", StagedAt: stage}
	if databases {
		result.DatabasesRestored, result.DatabaseErrors = restoreSQL(cfg, stage, user)
		if len(result.DatabaseErrors) > 0 {
			return result, fmt.Errorf("database restore completed with %d error(s)", len(result.DatabaseErrors))
		}
	}
	result.MailStaged, result.MailboxesStaged, result.MailErrors = restoreMail(cfg, stage, user)
	if len(result.MailErrors) > 0 {
		return result, fmt.Errorf("mail restore completed with %d error(s)", len(result.MailErrors))
	}
	return result, nil
}

// restoreMail preserves cPanel mailbox data and account mail metadata under a
// private StePanel root. Host-specific Exim/Dovecot configuration is not
// copied into /etc because it can break the destination mail server.
func restoreMail(cfg Config, stage, user string) (bool, []string, []string) {
	sourceMail := filepath.Join(stage, "homedir", "mail")
	sourceEtc := filepath.Join(stage, "homedir", "etc")
	if _, err := os.Stat(sourceMail); err != nil {
		return false, nil, nil
	}
	if cfg.MailRoot == "" {
		return false, nil, []string{"mail root is not configured; set STEPANEL_MAIL_ROOT"}
	}
	root := filepath.Join(cfg.MailRoot, user)
	if err := os.MkdirAll(root, 0700); err != nil {
		return false, nil, []string{"create mail root: " + err.Error()}
	}
	if err := copyTree(sourceMail, filepath.Join(root, "mail")); err != nil {
		return false, nil, []string{"copy mailbox data: " + err.Error()}
	}
	if _, err := os.Stat(sourceEtc); err == nil {
		if err := copyTree(sourceEtc, filepath.Join(root, "etc")); err != nil {
			return false, nil, []string{"copy mail metadata: " + err.Error()}
		}
	}
	mailboxes := []string{}
	mailRoot := filepath.Join(root, "mail")
	_ = filepath.Walk(mailRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && path != mailRoot {
			rel, relErr := filepath.Rel(mailRoot, path)
			if relErr == nil && strings.Count(filepath.ToSlash(rel), "/") == 1 {
				mailboxes = append(mailboxes, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	sort.Strings(mailboxes)
	return true, mailboxes, nil
}

func extractArchive(archive, destination string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !safeArchivePath(h.Name) {
			return errors.New("unsafe archive path")
		}
		target := filepath.Join(destination, filepath.Clean(h.Name))
		if !strings.HasPrefix(target, filepath.Clean(destination)+string(os.PathSeparator)) {
			return errors.New("archive escapes staging directory")
		}
		if h.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unsupported archive entry type: %s", h.Name)
		}
		if h.Size < 0 || h.Size > 2<<30 || total+h.Size > 20<<30 {
			return errors.New("archive contents exceed the 20 GiB extraction limit")
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(dst, io.LimitReader(tr, h.Size))
		dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if written != h.Size {
			return errors.New("archive entry is truncated")
		}
		total += written
	}
}
func restoreSQL(cfg Config, stage, user string) ([]string, []string) {
	matches, _ := filepath.Glob(filepath.Join(stage, "mysql", "*.sql"))
	restored, failures := []string{}, []string{}
	for _, dump := range matches {
		db := safeUser(strings.TrimSuffix(filepath.Base(dump), ".sql"))
		if db == "" {
			failures = append(failures, filepath.Base(dump)+": invalid database name")
			continue
		}
		name := user + "_" + db
		if len(name) > 64 {
			failures = append(failures, name+": database name exceeds 64 characters")
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		args := mysqlArgs(cfg)
		args = append(args, "--batch", "--execute", "CREATE DATABASE IF NOT EXISTS `"+name+"`")
		cmd := exec.CommandContext(ctx, "mysql", args...)
		if cfg.DBPassword != "" {
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			failures = append(failures, name+": create failed: "+strings.TrimSpace(string(output)))
			cancel()
			continue
		}
		input, err := os.Open(dump)
		if err != nil {
			failures = append(failures, name+": open failed: "+err.Error())
			cancel()
			continue
		}
		args = mysqlArgs(cfg)
		args = append(args, name)
		cmd = exec.CommandContext(ctx, "mysql", args...)
		if cfg.DBPassword != "" {
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.DBPassword)
		}
		cmd.Stdin = input
		output, err = cmd.CombinedOutput()
		input.Close()
		cancel()
		if err != nil {
			failures = append(failures, name+": import failed: "+strings.TrimSpace(string(output)))
			continue
		}
		restored = append(restored, name)
	}
	return restored, failures
}

func mysqlArgs(cfg Config) []string {
	args := []string{}
	if cfg.DBHost != "" {
		args = append(args, "--host", cfg.DBHost)
	}
	if cfg.DBUser != "" {
		args = append(args, "--user", cfg.DBUser)
	}
	return args
}
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if existing, statErr := os.Lstat(target); statErr == nil && existing.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination symlink is not allowed: %s", target)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_EXCL, 0640)
		if os.IsExist(err) {
			out, err = os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0640)
		}
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
		return err
	})
}
func firstExisting(paths ...string) string {
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}
func safeArchivePath(path string) bool {
	clean := filepath.Clean(path)
	return path != "" && !strings.Contains(path, "\\") && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !filepath.IsAbs(path)
}
