package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxBackupBytes int64 = 20 << 30

type BackupEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type BackupManifest struct {
	Version       int           `json:"version"`
	Site          string        `json:"site"`
	CreatedAt     time.Time     `json:"created_at"`
	VerifiedAt    time.Time     `json:"verified_at"`
	Archive       string        `json:"archive"`
	ArchiveSHA256 string        `json:"archive_sha256"`
	Bytes         int64         `json:"bytes"`
	Databases     []string      `json:"databases"`
	Entries       []BackupEntry `json:"entries"`
}

type BackupResult struct {
	Site          string    `json:"site"`
	Path          string    `json:"path"`
	ArchiveSHA256 string    `json:"archive_sha256"`
	Bytes         int64     `json:"bytes"`
	Databases     []string  `json:"databases"`
	VerifiedAt    time.Time `json:"verified_at"`
}

func (a *App) backups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		manifests, err := listBackups(a.Config.BackupRoot)
		if err != nil {
			http.Error(w, "unable to inspect backups", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"backups": manifests})
	case http.MethodPost:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		var input struct {
			Site             string `json:"site"`
			IncludeDatabases bool   `json:"include_databases"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		input.Site = safeUser(input.Site)
		if input.Site == "" {
			http.Error(w, "invalid site", http.StatusUnprocessableEntity)
			return
		}
		if input.IncludeDatabases && a.Config.DBCtl == "" {
			http.Error(w, "managed database backup requires the local database helper", http.StatusUnprocessableEntity)
			return
		}
		publicRoot := filepath.Join(a.Config.WebRoot, "sites", input.Site, "public")
		if info, err := os.Stat(publicRoot); err != nil || !info.IsDir() {
			http.Error(w, "site document root does not exist", http.StatusUnprocessableEntity)
			return
		}
		if err := os.MkdirAll(a.Config.BackupRoot, 0750); err != nil {
			http.Error(w, "backup root is unavailable", http.StatusInternalServerError)
			return
		}
		if free, err := availableBytes(a.Config.BackupRoot); err == nil && free < a.Config.MinFreeBytes {
			http.Error(w, "insufficient free space for backup", http.StatusInsufficientStorage)
			return
		}
		jobID := "backup-" + time.Now().UTC().Format("20060102-150405.000000000") + "-" + input.Site
		if err := a.Jobs.SubmitBackup(jobID, input.Site, func() (BackupResult, error) {
			result, err := CreateSiteBackup(a.Config, input.Site, input.IncludeDatabases)
			if err != nil {
				_ = Audit(a.Config.AuditLog, "site.backup.failed", input.Site, err.Error())
			} else {
				_ = Audit(a.Config.AuditLog, "site.backup.completed", input.Site, result.ArchiveSHA256)
			}
			return result, err
		}); err != nil {
			if errors.Is(err, ErrJobBusy) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
			} else {
				http.Error(w, "could not persist backup job", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status_url": filepath.Join("/api/jobs", jobID)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func CreateSiteBackup(cfg Config, site string, includeDatabases bool) (result BackupResult, returnErr error) {
	if safeUser(site) == "" {
		return result, errors.New("invalid backup site")
	}
	publicRoot := filepath.Join(cfg.WebRoot, "sites", site, "public")
	if err := ensureInside(cfg.WebRoot, publicRoot); err != nil {
		return result, err
	}
	if err := os.MkdirAll(cfg.BackupRoot, 0750); err != nil {
		return result, err
	}
	tempDir, err := os.MkdirTemp(cfg.BackupRoot, ".backup-")
	if err != nil {
		return result, err
	}
	defer func() {
		if returnErr != nil {
			_ = os.RemoveAll(tempDir)
		}
	}()
	if err := os.Chmod(tempDir, 0700); err != nil {
		return result, err
	}
	archivePath := filepath.Join(tempDir, "backup.tar.gz")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	gz := gzip.NewWriter(archive)
	tw := tar.NewWriter(gz)
	manifest := BackupManifest{Version: 1, Site: site, CreatedAt: time.Now().UTC(), Archive: "backup.tar.gz", Databases: []string{}, Entries: []BackupEntry{}}
	var uncompressedBytes int64
	closeArchive := func() error {
		if err := tw.Close(); err != nil {
			_ = gz.Close()
			_ = archive.Close()
			return err
		}
		if err := gz.Close(); err != nil {
			_ = archive.Close()
			return err
		}
		if err := archive.Sync(); err != nil {
			_ = archive.Close()
			return err
		}
		return archive.Close()
	}
	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000000
	}
	if err := addBackupTree(tw, publicRoot, "site/public", maxEntries, &uncompressedBytes, &manifest); err != nil {
		_ = closeArchive()
		return result, err
	}
	if includeDatabases {
		databases, err := managedDatabasesForSite(cfg, site)
		if err != nil {
			_ = closeArchive()
			return result, err
		}
		for _, database := range databases {
			if len(manifest.Entries) >= maxEntries {
				_ = closeArchive()
				return result, errors.New("backup contains too many entries")
			}
			dumpPath := filepath.Join(tempDir, database+".sql")
			if err := dumpManagedDatabase(cfg, database, dumpPath); err != nil {
				_ = closeArchive()
				return result, err
			}
			if err := addBackupFile(tw, dumpPath, "databases/"+database+".sql", &uncompressedBytes, &manifest); err != nil {
				_ = closeArchive()
				return result, err
			}
			if err := os.Remove(dumpPath); err != nil {
				_ = closeArchive()
				return result, err
			}
			manifest.Databases = append(manifest.Databases, database)
		}
	}
	if err := closeArchive(); err != nil {
		return result, err
	}
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return result, err
	}
	manifest.Bytes = archiveInfo.Size()
	manifest.ArchiveSHA256, err = fileSHA256(archivePath)
	if err != nil {
		return result, err
	}
	if err := VerifyBackupArchive(archivePath, manifest); err != nil {
		return result, fmt.Errorf("verify completed backup: %w", err)
	}
	manifest.VerifiedAt = time.Now().UTC()
	if err := writeBackupManifest(tempDir, manifest); err != nil {
		return result, err
	}
	if err := writeSyncedFile(filepath.Join(tempDir, "backup.tar.gz.sha256"), []byte(manifest.ArchiveSHA256+"  backup.tar.gz\n"), 0600); err != nil {
		return result, err
	}
	if err := syncDirectory(tempDir); err != nil {
		return result, err
	}
	finalName := manifest.CreatedAt.Format("20060102-150405.000000000") + "-" + site
	finalPath := filepath.Join(cfg.BackupRoot, finalName)
	if err := os.Rename(tempDir, finalPath); err != nil {
		return result, err
	}
	if err := syncDirectory(cfg.BackupRoot); err != nil {
		_ = os.Rename(finalPath, tempDir)
		return result, err
	}
	result = BackupResult{Site: site, Path: finalPath, ArchiveSHA256: manifest.ArchiveSHA256, Bytes: manifest.Bytes, Databases: manifest.Databases, VerifiedAt: manifest.VerifiedAt}
	return result, nil
}

func addBackupTree(tw *tar.Writer, root, prefix string, maxEntries int, totalBytes *int64, manifest *BackupManifest) error {
	entries := 0
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > maxEntries {
			return errors.New("backup contains too many filesystem entries")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup refuses symlink %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := prefix
		if rel != "." {
			name = filepath.ToSlash(filepath.Join(prefix, rel))
		}
		if info.IsDir() {
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = name + "/"
			header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
			return tw.WriteHeader(header)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup refuses special file %s", path)
		}
		return addBackupFile(tw, path, name, totalBytes, manifest)
	})
}

func addBackupFile(tw *tar.Writer, source, name string, totalBytes *int64, manifest *BackupManifest) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.Size() < 0 || info.Size() > 2<<30 {
		return fmt.Errorf("backup entry %s exceeds the 2 GiB restore limit", name)
	}
	if *totalBytes+info.Size() > maxBackupBytes {
		return errors.New("backup exceeds the 20 GiB restore limit")
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.CopyN(io.MultiWriter(tw, hash), file, info.Size())
	if err != nil {
		return err
	}
	manifest.Entries = append(manifest.Entries, BackupEntry{Path: header.Name, Size: written, SHA256: hex.EncodeToString(hash.Sum(nil))})
	*totalBytes += written
	return nil
}

func managedDatabasesForSite(cfg Config, site string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := helperCommandContext(ctx, cfg, cfg.DBCtl, "list", site).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list managed databases: %w: %s", err, strings.TrimSpace(string(output)))
	}
	databases := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		name := strings.SplitN(line, "\t", 2)[0]
		if !validManagedDatabaseIdentifier(name, 64) {
			return nil, errors.New("database helper returned an invalid managed database")
		}
		databases = append(databases, name)
	}
	sort.Strings(databases)
	return databases, nil
}

func dumpManagedDatabase(cfg Config, database, destination string) error {
	if !validManagedDatabaseIdentifier(database, 64) {
		return errors.New("invalid managed database")
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := helperCommandContext(ctx, cfg, cfg.DBCtl, "dump", database)
	var stderr strings.Builder
	cmd.Stdout = out
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr == nil {
		runErr = out.Sync()
	}
	if closeErr := out.Close(); runErr == nil {
		runErr = closeErr
	}
	if runErr != nil {
		return fmt.Errorf("dump managed database %s: %w: %s", database, runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func VerifyBackupArchive(path string, manifest BackupManifest) error {
	if manifest.Version != 1 || safeUser(manifest.Site) == "" || manifest.Archive != "backup.tar.gz" || len(manifest.ArchiveSHA256) != sha256.Size*2 {
		return errors.New("invalid backup manifest metadata")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != manifest.Bytes {
		return errors.New("archive size does not match manifest")
	}
	archiveHash, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if archiveHash != manifest.ArchiveSHA256 {
		return errors.New("archive checksum does not match manifest")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	expected := make(map[string]BackupEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if _, err := hex.DecodeString(entry.SHA256); err != nil || len(entry.SHA256) != sha256.Size*2 {
			return errors.New("manifest contains an invalid entry checksum")
		}
		if _, exists := expected[entry.Path]; exists || !safeArchivePath(entry.Path) || entry.Size < 0 {
			return errors.New("manifest contains a duplicate or unsafe entry")
		}
		expected[entry.Path] = entry
	}
	seen := make(map[string]bool, len(expected))
	tr := tar.NewReader(gz)
	var total int64
	for count := 0; ; count++ {
		if count > 1000000 {
			return errors.New("backup archive contains too many entries")
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !safeArchivePath(header.Name) {
			return errors.New("backup archive contains an unsafe path")
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return errors.New("backup archive contains an unsupported entry type")
		}
		entry, ok := expected[header.Name]
		if !ok || seen[header.Name] || entry.Size != header.Size {
			return fmt.Errorf("backup entry %s is unexpected or has the wrong size", header.Name)
		}
		total += header.Size
		if total > maxBackupBytes {
			return errors.New("backup archive exceeds the verification limit")
		}
		hash := sha256.New()
		if copied, err := io.Copy(hash, tr); err != nil || copied != header.Size {
			return fmt.Errorf("read backup entry %s: %w", header.Name, err)
		}
		if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return fmt.Errorf("backup entry %s checksum mismatch", header.Name)
		}
		seen[header.Name] = true
	}
	if len(seen) != len(expected) {
		return errors.New("backup archive is missing manifest entries")
	}
	return nil
}

func writeBackupManifest(root string, manifest BackupManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(root, ".manifest-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempName, filepath.Join(root, "manifest.json"))
}

func writeSyncedFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	err = directory.Sync()
	if closeErr := directory.Close(); err == nil {
		err = closeErr
	}
	return err
}

func readBackupManifest(root string) (BackupManifest, error) {
	path := filepath.Join(root, "manifest.json")
	info, err := os.Stat(path)
	if err != nil {
		return BackupManifest{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<20 {
		return BackupManifest{}, errors.New("backup manifest is not a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return BackupManifest{}, err
	}
	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Version != 1 || safeUser(manifest.Site) == "" || manifest.Archive != "backup.tar.gz" {
		return BackupManifest{}, errors.New("invalid backup manifest")
	}
	return manifest, nil
}

func VerifySiteBackup(root string) (BackupManifest, error) {
	manifest, err := readBackupManifest(root)
	if err != nil {
		return BackupManifest{}, err
	}
	if err := VerifyBackupArchive(filepath.Join(root, manifest.Archive), manifest); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

func listBackups(root string) ([]BackupResult, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := []BackupResult{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		manifest, err := readBackupManifest(path)
		if err != nil {
			return nil, fmt.Errorf("invalid backup manifest %s", entry.Name())
		}
		backups = append(backups, BackupResult{Site: manifest.Site, Path: path, ArchiveSHA256: manifest.ArchiveSHA256, Bytes: manifest.Bytes, Databases: manifest.Databases, VerifiedAt: manifest.VerifiedAt})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].VerifiedAt.After(backups[j].VerifiedAt) })
	return backups, nil
}
