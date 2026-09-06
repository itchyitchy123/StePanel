package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	jobadmission "github.com/itchyitchy123/StePanel/internal/jobs"
)

var ErrJobBusy = errors.New("too many long-running jobs or target is already active")

const maxJobStateBytes = 16 << 20

func validJobOperationKey(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:-", r) {
			continue
		}
		return false
	}
	return true
}

func newJobID(kind string) (string, error) {
	random, err := randomSecret()
	if err != nil {
		return "", err
	}
	return kind + "-" + random, nil
}

type Job struct {
	ID           string               `json:"id"`
	Kind         string               `json:"kind"`
	OperationKey string               `json:"operation_key,omitempty"`
	State        string               `json:"state"`
	User         string               `json:"user"`
	Result       *ImportResult        `json:"result,omitempty"`
	WPress       *WPressResult        `json:"wpress,omitempty"`
	Certificate  *CertificateResult   `json:"certificate,omitempty"`
	Backup       *BackupResult        `json:"backup,omitempty"`
	Restore      *BackupRestoreResult `json:"restore,omitempty"`
	Cloud        *CloudActionResult   `json:"cloud,omitempty"`
	Error        string               `json:"error,omitempty"`
	StartedAt    time.Time            `json:"started_at"`
	FinishedAt   *time.Time           `json:"finished_at,omitempty"`
}

func (j *Jobs) SubmitCloud(id, target string, work func() (CloudActionResult, error)) error {
	j.admission.RLock()
	defer j.admission.RUnlock()
	if !j.reserve(target) {
		return ErrJobBusy
	}
	j.wg.Add(1)
	item := &Job{ID: id, Kind: "cloud.action", State: "running", User: target, StartedAt: time.Now().UTC()}
	if err := j.add(item); err != nil {
		j.wg.Done()
		j.release(target)
		return fmt.Errorf("persist queued job: %w", err)
	}
	go func() {
		defer j.wg.Done()
		defer j.release(target)
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Cloud = &result
		}
		j.mu.Unlock()
		j.complete(item)
	}()
	return nil
}

type jobStore struct {
	Version int    `json:"version"`
	Jobs    []*Job `json:"jobs"`
}

type Jobs struct {
	mu            sync.RWMutex
	items         map[string]*Job
	admissionGate *jobadmission.Admission
	activeDomains map[string]bool
	activeTargets map[string]bool
	wg            sync.WaitGroup
	admission     sync.RWMutex
	path          string
	persistErr    error
}

func NewJobs() *Jobs { return newJobs("") }

func OpenJobs(path string, limits ...int) (*Jobs, error) {
	limit := 2
	if len(limits) > 0 && limits[0] > 0 {
		limit = limits[0]
	}
	jobs := newJobs(path, limit)
	if err := jobs.load(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func newJobs(path string, limits ...int) *Jobs {
	limit := 2
	if len(limits) > 0 && limits[0] > 0 {
		limit = limits[0]
	}
	return &Jobs{
		items:         make(map[string]*Job),
		admissionGate: jobadmission.NewAdmission(limit),
		activeDomains: make(map[string]bool),
		activeTargets: make(map[string]bool),
		path:          path,
	}
}

func (j *Jobs) load() error {
	if j.path == "" {
		return nil
	}
	data, err := os.ReadFile(j.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read job state: %w", err)
	}
	if len(data) > maxJobStateBytes {
		return errors.New("job state exceeds 16 MiB")
	}
	var store jobStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("decode job state: %w", err)
	}
	if store.Version != 1 {
		return fmt.Errorf("unsupported job state version %d", store.Version)
	}
	now := time.Now().UTC()
	reconciled := false
	for _, item := range store.Jobs {
		if item == nil || item.ID == "" {
			return errors.New("job state contains an invalid job")
		}
		if _, exists := j.items[item.ID]; exists {
			return fmt.Errorf("job state contains duplicate ID %q", item.ID)
		}
		if item.State == "running" {
			item.State = "failed"
			item.Error = "interrupted by an unclean shutdown; verify restore rollback and destination integrity"
			item.FinishedAt = &now
			reconciled = true
		}
		j.items[item.ID] = item
	}
	if reconciled {
		if err := j.persistLocked(); err != nil {
			return fmt.Errorf("persist reconciled job state: %w", err)
		}
	}
	return nil
}

func (j *Jobs) persistLocked() error {
	if j.path == "" {
		return nil
	}
	root := filepath.Dir(j.path)
	if err := os.MkdirAll(root, 0750); err != nil {
		return fmt.Errorf("create job state directory: %w", err)
	}
	ids := make([]string, 0, len(j.items))
	for id := range j.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	store := jobStore{Version: 1, Jobs: make([]*Job, 0, len(ids))}
	for _, id := range ids {
		store.Jobs = append(store.Jobs, j.items[id])
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode job state: %w", err)
	}
	if len(data)+1 > maxJobStateBytes {
		return errors.New("job state exceeds 16 MiB; prune completed jobs before retrying")
	}
	tmp, err := os.CreateTemp(root, ".jobs-*.tmp")
	if err != nil {
		return fmt.Errorf("create job state: %w", err)
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
		return fmt.Errorf("write job state: %w", err)
	}
	if err := os.Rename(tmpName, j.path); err != nil {
		return fmt.Errorf("replace job state: %w", err)
	}
	if dir, err := os.Open(root); err == nil {
		syncErr := dir.Sync()
		_ = dir.Close()
		if syncErr != nil {
			return fmt.Errorf("sync job state directory: %w", syncErr)
		}
	}
	return nil
}

func (j *Jobs) add(item *Job) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, exists := j.items[item.ID]; exists {
		return fmt.Errorf("job ID %q already exists", item.ID)
	}
	j.items[item.ID] = item
	if err := j.persistLocked(); err != nil {
		j.persistErr = err
		delete(j.items, item.ID)
		return err
	}
	j.persistErr = nil
	return nil
}

func (j *Jobs) complete(item *Job) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.persistLocked(); err != nil {
		j.persistErr = err
		log.Printf("persist completed job %s: %v", item.ID, err)
	} else {
		j.persistErr = nil
	}
}

func (j *Jobs) PersistenceError() error {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.persistErr
}

func (j *Jobs) SubmitWPress(id, user string, work func() (WPressResult, error)) error {
	j.admission.RLock()
	defer j.admission.RUnlock()
	if !j.reserve(user) {
		return ErrJobBusy
	}
	j.wg.Add(1)
	item := &Job{ID: id, Kind: "wordpress.restore", State: "running", User: user, StartedAt: time.Now().UTC()}
	if err := j.add(item); err != nil {
		j.wg.Done()
		j.release(user)
		return fmt.Errorf("persist queued job: %w", err)
	}
	go func() {
		defer j.wg.Done()
		defer j.release(user)
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.WPress = &result
		}
		j.mu.Unlock()
		j.complete(item)
	}()
	return nil
}

func (j *Jobs) Get(id string) (Job, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	item, ok := j.items[id]
	if !ok {
		return Job{}, false
	}
	copy := *item
	return copy, true
}

func (j *Jobs) List(limit int) []Job {
	if limit < 1 {
		limit = 50
	}
	j.mu.RLock()
	items := make([]Job, 0, len(j.items))
	for _, item := range j.items {
		if item != nil {
			items = append(items, *item)
		}
	}
	j.mu.RUnlock()
	sort.Slice(items, func(i, k int) bool { return items[i].StartedAt.After(items[k].StartedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (j *Jobs) Submit(id, user string, work func() (ImportResult, error)) error {
	_, _, err := j.submitImport(id, user, "", work)
	return err
}

// SubmitIdempotent queues a cpmove restore with a caller-supplied operation
// key. Reusing the same key for the same account returns the original job ID
// and does not execute the restore a second time. The key is persisted with
// the job so this remains true after a control-plane restart.
func (j *Jobs) SubmitIdempotent(id, user, operationKey string, work func() (ImportResult, error)) (jobID string, existing bool, err error) {
	return j.submitImport(id, user, operationKey, work)
}

func (j *Jobs) submitImport(id, user, operationKey string, work func() (ImportResult, error)) (jobID string, existing bool, err error) {
	j.admission.RLock()
	defer j.admission.RUnlock()
	existingID, reserved := j.reserveOperation(user, "cpmove.restore", operationKey)
	if existingID != "" {
		return existingID, true, nil
	}
	if !reserved {
		return "", false, ErrJobBusy
	}
	j.wg.Add(1)
	item := &Job{ID: id, Kind: "cpmove.restore", OperationKey: operationKey, State: "running", User: user, StartedAt: time.Now().UTC()}
	if err := j.add(item); err != nil {
		j.wg.Done()
		j.release(user)
		return "", false, fmt.Errorf("persist queued job: %w", err)
	}
	go func() {
		defer j.wg.Done()
		defer j.release(user)
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Result = &result
		}
		j.mu.Unlock()
		j.complete(item)
	}()
	return id, false, nil
}

func (j *Jobs) SubmitBackup(id, site string, work func() (BackupResult, error)) error {
	j.admission.RLock()
	defer j.admission.RUnlock()
	if !j.reserve(site) {
		return ErrJobBusy
	}
	j.wg.Add(1)
	item := &Job{ID: id, Kind: "site.backup", State: "running", User: site, StartedAt: time.Now().UTC()}
	if err := j.add(item); err != nil {
		j.wg.Done()
		j.release(site)
		return fmt.Errorf("persist queued job: %w", err)
	}
	go func() {
		defer j.wg.Done()
		defer j.release(site)
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Backup = &result
		}
		j.mu.Unlock()
		j.complete(item)
	}()
	return nil
}

// SubmitBackupRestore queues a destructive, journaled site restore while
// using the same per-site admission guard as backup creation.
func (j *Jobs) SubmitBackupRestore(id, site string, work func() (BackupRestoreResult, error)) error {
	j.admission.RLock()
	defer j.admission.RUnlock()
	if !j.reserve(site) {
		return ErrJobBusy
	}
	j.wg.Add(1)
	item := &Job{ID: id, Kind: "site.backup-restore", State: "running", User: site, StartedAt: time.Now().UTC()}
	if err := j.add(item); err != nil {
		j.wg.Done()
		j.release(site)
		return fmt.Errorf("persist queued job: %w", err)
	}
	go func() {
		defer j.wg.Done()
		defer j.release(site)
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Restore = &result
		}
		j.mu.Unlock()
		j.complete(item)
	}()
	return nil
}

func (j *Jobs) reserve(target string) bool {
	_, reserved := j.reserveOperation(target, "", "")
	return reserved
}

// reserveOperation combines admission and idempotency lookup while holding
// the job lock after acquiring a slot. This closes the race where two retrying
// requests could both observe no matching operation before either persisted.
func (j *Jobs) reserveOperation(target, kind, operationKey string) (existingID string, reserved bool) {
	if !j.admissionGate.Acquire() {
		return "", false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if operationKey != "" {
		for _, item := range j.items {
			if item != nil && item.Kind == kind && item.User == target && item.OperationKey == operationKey {
				j.admissionGate.Release()
				return item.ID, false
			}
		}
	}
	if j.activeTargets[target] {
		j.admissionGate.Release()
		return "", false
	}
	j.activeTargets[target] = true
	return "", true
}

func (j *Jobs) release(target string) {
	j.mu.Lock()
	delete(j.activeTargets, target)
	j.mu.Unlock()
	j.admissionGate.Release()
}

func (j *Jobs) SubmitCertificate(id, domain string, work func() (CertificateResult, error)) error {
	j.admission.RLock()
	defer j.admission.RUnlock()
	if !j.admissionGate.Acquire() {
		return ErrJobBusy
	}
	j.mu.Lock()
	if j.activeDomains[domain] {
		j.mu.Unlock()
		j.admissionGate.Release()
		return ErrJobBusy
	}
	j.activeDomains[domain] = true
	j.mu.Unlock()
	j.wg.Add(1)
	item := &Job{ID: id, Kind: "certificate.issue", State: "running", User: domain, StartedAt: time.Now().UTC()}
	if err := j.add(item); err != nil {
		j.wg.Done()
		j.mu.Lock()
		delete(j.activeDomains, domain)
		j.mu.Unlock()
		j.admissionGate.Release()
		return fmt.Errorf("persist queued job: %w", err)
	}
	go func() {
		defer j.wg.Done()
		defer func() { j.mu.Lock(); delete(j.activeDomains, domain); j.mu.Unlock(); j.admissionGate.Release() }()
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Certificate = &result
		}
		j.mu.Unlock()
		j.complete(item)
	}()
	return nil
}

func (j *Jobs) Wait(ctx context.Context) error {
	j.admission.Lock()
	defer j.admission.Unlock()
	done := make(chan struct{})
	go func() {
		j.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (j *Jobs) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	j.mu.Lock()
	defer j.mu.Unlock()
	changed := false
	for id, item := range j.items {
		if item.FinishedAt != nil && item.FinishedAt.Before(cutoff) {
			delete(j.items, id)
			changed = true
		}
	}
	if changed {
		if err := j.persistLocked(); err != nil {
			j.persistErr = err
			log.Printf("persist job cleanup: %v", err)
		} else {
			j.persistErr = nil
		}
	}
}
