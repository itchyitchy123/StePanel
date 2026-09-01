package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SiteTransaction struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Site        string    `json:"site"`
	Home        string    `json:"home"`
	Backup      string    `json:"backup"`
	FailedSite  string    `json:"failed_site,omitempty"`
	HadExisting bool      `json:"had_existing"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	dir string
}

func BeginSiteTransaction(root, home, kind, site string) (*SiteTransaction, error) {
	if safeUser(site) == "" {
		return nil, errors.New("invalid recovery site")
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("site recovery root is not configured")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create recovery root: %w", err)
	}
	dir, err := os.MkdirTemp(root, time.Now().UTC().Format("20060102-150405-")+site+"-")
	if err != nil {
		return nil, fmt.Errorf("create recovery transaction: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	now := time.Now().UTC()
	txn := &SiteTransaction{
		Version:   1,
		ID:        filepath.Base(dir),
		Kind:      kind,
		Site:      site,
		Home:      home,
		Backup:    filepath.Join(dir, "site-before"),
		State:     "prepared",
		CreatedAt: now,
		UpdatedAt: now,
		dir:       dir,
	}
	if existing, statErr := os.Lstat(home); statErr == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			_ = os.RemoveAll(dir)
			return nil, errors.New("destination site is a symlink")
		}
		txn.HadExisting = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("inspect destination site: %w", statErr)
	}
	if err := txn.persist(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if txn.HadExisting {
		if err := os.Rename(home, txn.Backup); err != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("snapshot existing site: %w", err)
		}
		txn.State = "site-backed-up"
		if err := txn.persist(); err != nil {
			if rollbackErr := os.Rename(txn.Backup, home); rollbackErr != nil {
				return nil, fmt.Errorf("persist recovery transaction: %w (restore backup: %v)", err, rollbackErr)
			}
			_ = os.RemoveAll(dir)
			return nil, err
		}
	}
	return txn, nil
}

func (t *SiteTransaction) Commit() error {
	t.State = "committed"
	return t.persist()
}

func (t *SiteTransaction) Rollback() error {
	if t.State == "committed" || t.State == "rolled-back" {
		return nil
	}
	if t.State == "rolling-back" && t.HadExisting {
		_, backupErr := os.Lstat(t.Backup)
		_, homeErr := os.Lstat(t.Home)
		if errors.Is(backupErr, os.ErrNotExist) && homeErr == nil {
			t.State = "rolled-back"
			return t.persist()
		}
	}
	t.State = "rolling-back"
	if err := t.persist(); err != nil {
		return fmt.Errorf("record rollback start: %w", err)
	}
	if _, err := os.Lstat(t.Home); err == nil {
		failed := filepath.Join(t.dir, "failed-site")
		if _, existsErr := os.Lstat(failed); existsErr == nil {
			failed = filepath.Join(t.dir, "failed-site-"+time.Now().UTC().Format("150405.000000000"))
		}
		if err := os.Rename(t.Home, failed); err != nil {
			return fmt.Errorf("preserve failed site: %w", err)
		}
		t.FailedSite = failed
		if err := t.persist(); err != nil {
			return fmt.Errorf("record failed site: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect failed site: %w", err)
	}
	if t.HadExisting {
		if _, err := os.Lstat(t.Backup); err != nil {
			return fmt.Errorf("recovery backup is unavailable: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(t.Home), 0750); err != nil {
			return err
		}
		if err := os.Rename(t.Backup, t.Home); err != nil {
			return fmt.Errorf("restore previous site: %w", err)
		}
	}
	t.State = "rolled-back"
	return t.persist()
}

func (t *SiteTransaction) persist() error {
	t.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(t.dir, "transaction.json")
	tmp, err := os.CreateTemp(t.dir, ".transaction-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(data, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if dir, err := os.Open(t.dir); err == nil {
		syncErr := dir.Sync()
		_ = dir.Close()
		if syncErr != nil {
			return syncErr
		}
	}
	return nil
}

func RecoverSiteTransactions(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	recovered := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		txn, err := loadSiteTransaction(dir)
		if err != nil {
			return recovered, err
		}
		if txn.State == "committed" || txn.State == "rolled-back" {
			continue
		}
		if err := txn.Rollback(); err != nil {
			return recovered, fmt.Errorf("recover transaction %s: %w", txn.ID, err)
		}
		recovered = append(recovered, txn.ID)
	}
	sort.Strings(recovered)
	return recovered, nil
}

func loadSiteTransaction(dir string) (*SiteTransaction, error) {
	data, err := os.ReadFile(filepath.Join(dir, "transaction.json"))
	if err != nil {
		return nil, fmt.Errorf("read recovery transaction %s: %w", filepath.Base(dir), err)
	}
	var txn SiteTransaction
	if err := json.Unmarshal(data, &txn); err != nil {
		return nil, fmt.Errorf("decode recovery transaction %s: %w", filepath.Base(dir), err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if txn.Version != 1 || txn.ID != filepath.Base(dir) || safeUser(txn.Site) == "" || !filepath.IsAbs(txn.Home) {
		return nil, fmt.Errorf("invalid recovery transaction %s", filepath.Base(dir))
	}
	expectedBackup := filepath.Join(dir, "site-before")
	if filepath.Clean(txn.Backup) != expectedBackup || txn.FailedSite != "" && !strings.HasPrefix(filepath.Clean(txn.FailedSite), dir+string(os.PathSeparator)) {
		return nil, fmt.Errorf("recovery transaction %s contains unsafe paths", txn.ID)
	}
	txn.dir = dir
	return &txn, nil
}

func CleanupSiteTransactions(root string, maxAge time.Duration) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		txn, err := loadSiteTransaction(dir)
		if err != nil {
			return err
		}
		if (txn.State == "committed" || txn.State == "rolled-back") && txn.UpdatedAt.Before(cutoff) {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
		}
	}
	return nil
}
