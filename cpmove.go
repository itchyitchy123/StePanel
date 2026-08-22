package main

import (
    "archive/tar"
    "compress/gzip"
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

type CPMoveInfo struct { Archive string `json:"archive"`; Entries int `json:"entries"`; User string `json:"detected_user"`; HasHome bool `json:"has_home"`; HasMySQL bool `json:"has_mysql"`; Databases []string `json:"databases"` }
type ImportResult struct { User, Home string `json:"user"`; FilesRestored bool `json:"files_restored"`; DatabasesRestored []string `json:"databases_restored"`; StagedAt string `json:"staged_at"` }

func InspectCPMove(file multipart.File, header *multipart.FileHeader) (CPMoveInfo, error) {
    if header.Size > 20<<30 { return CPMoveInfo{}, errors.New("backup exceeds the 20 GiB upload limit") }
    if _, err := file.Seek(0, io.SeekStart); err != nil { return CPMoveInfo{}, err }; gz, err := gzip.NewReader(file); if err != nil { return CPMoveInfo{}, errors.New("backup is not a valid gzip archive") }; defer gz.Close()
    tr := tar.NewReader(gz); info := CPMoveInfo{Archive: header.Filename}; seen := map[string]bool{}
    for { h, err := tr.Next(); if errors.Is(err, io.EOF) { break }; if err != nil { return info, errors.New("backup tar stream is damaged") }; if !safeArchivePath(h.Name) { return info, fmt.Errorf("unsafe archive path: %s", h.Name) }; info.Entries++; name := strings.Trim(h.Name, "/"); parts := strings.Split(name, "/"); if len(parts) > 1 && parts[0] == "homedir" { info.HasHome = true }; if strings.HasPrefix(name, "mysql/") || strings.Contains(name, "/mysql/") { info.HasMySQL = true; if strings.HasSuffix(name, ".sql") { db := strings.TrimSuffix(filepath.Base(name), ".sql"); if !seen[db] { info.Databases = append(info.Databases, db); seen[db] = true } } }; if info.User == "" && strings.HasPrefix(name, "homedir/") && len(parts) > 1 { info.User = parts[1] } }
    sort.Strings(info.Databases); if info.Entries == 0 { return info, errors.New("backup archive is empty") }; return info, nil
}

func RestoreCPMove(cfg Config, file multipart.File, header *multipart.FileHeader, user string, databases bool) (ImportResult, error) {
    if _, err := file.Seek(0, io.SeekStart); err != nil { return ImportResult{}, err }; if _, err := InspectCPMove(file, header); err != nil { return ImportResult{}, err }; if _, err := file.Seek(0, io.SeekStart); err != nil { return ImportResult{}, err }
    id := time.Now().UTC().Format("20060102-150405") + "-" + user; stage := filepath.Join(cfg.ImportRoot, id); if err := os.MkdirAll(stage, 0700); err != nil { return ImportResult{}, err }; archive := filepath.Join(stage, "backup.tar.gz"); out, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600); if err != nil { return ImportResult{}, err }; if _, err = io.Copy(out, file); err != nil { out.Close(); return ImportResult{}, err }; out.Close(); if err = extractArchive(archive, stage); err != nil { return ImportResult{}, err }
    home := filepath.Join(cfg.WebRoot, "sites", user, "public"); if err = os.MkdirAll(home, 0750); err != nil { return ImportResult{}, err }; if source := firstExisting(filepath.Join(stage, "homedir", "public_html"), filepath.Join(stage, "homedir", user, "public_html")); source != "" { if err = copyTree(source, home); err != nil { return ImportResult{}, err } }; result := ImportResult{User: user, Home: home, FilesRestored: true, StagedAt: stage}; if databases { result.DatabasesRestored = restoreSQL(stage, user) }; return result, nil
}

func extractArchive(archive, destination string) error { f, err := os.Open(archive); if err != nil { return err }; defer f.Close(); gz, err := gzip.NewReader(f); if err != nil { return err }; defer gz.Close(); tr := tar.NewReader(gz); var total int64; for { h, err := tr.Next(); if errors.Is(err, io.EOF) { return nil }; if err != nil { return err }; if !safeArchivePath(h.Name) { return errors.New("unsafe archive path") }; target := filepath.Join(destination, filepath.Clean(h.Name)); if !strings.HasPrefix(target, filepath.Clean(destination)+string(os.PathSeparator)) { return errors.New("archive escapes staging directory") }; if h.FileInfo().IsDir() { if err = os.MkdirAll(target, 0700); err != nil { return err }; continue }; if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA { return fmt.Errorf("unsupported archive entry type: %s", h.Name) }; if h.Size < 0 || h.Size > 2<<30 || total+h.Size > 20<<30 { return errors.New("archive contents exceed the 20 GiB extraction limit") }; if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil { return err }; dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600); if err != nil { return err }; written, copyErr := io.Copy(dst, io.LimitReader(tr, h.Size)); dst.Close(); if copyErr != nil { return copyErr }; if written != h.Size { return errors.New("archive entry is truncated") }; total += written } }
func restoreSQL(stage, user string) []string { matches, _ := filepath.Glob(filepath.Join(stage, "mysql", "*.sql")); restored := []string{}; for _, dump := range matches { db := safeUser(strings.TrimSuffix(filepath.Base(dump), ".sql")); if db == "" { continue }; name := user + "_" + db; cmd := exec.Command("mysql", "--batch", "--execute", "CREATE DATABASE IF NOT EXISTS `"+name+"`"); if cmd.Run() != nil { continue }; input, err := os.Open(dump); if err != nil { continue }; cmd = exec.Command("mysql", name); cmd.Stdin = input; if cmd.Run() == nil { restored = append(restored, name) }; input.Close() }; return restored }
func copyTree(src, dst string) error { return filepath.Walk(src, func(path string, info os.FileInfo, err error) error { if err != nil { return err }; rel, err := filepath.Rel(src, path); if err != nil { return err }; target := filepath.Join(dst, rel); if info.IsDir() { return os.MkdirAll(target, 0750) }; in, err := os.Open(path); if err != nil { return err }; defer in.Close(); out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640); if err != nil { return err }; _, err = io.Copy(out, in); out.Close(); return err }) }
func firstExisting(paths ...string) string { for _, path := range paths { if info, err := os.Stat(path); err == nil && info.IsDir() { return path } }; return "" }
func safeArchivePath(path string) bool { clean := filepath.Clean(path); return path != "" && !strings.Contains(path, "\\") && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !filepath.IsAbs(path) }
