package main

import (
	"sync"
	"time"
)

type Job struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	State       string             `json:"state"`
	User        string             `json:"user"`
	Result      *ImportResult      `json:"result,omitempty"`
	WPress      *WPressResult      `json:"wpress,omitempty"`
	Certificate *CertificateResult `json:"certificate,omitempty"`
	Error       string             `json:"error,omitempty"`
	StartedAt   time.Time          `json:"started_at"`
	FinishedAt  *time.Time         `json:"finished_at,omitempty"`
}

func (j *Jobs) SubmitWPress(id, user string, work func() (WPressResult, error)) bool {
	select {
	case j.slots <- struct{}{}:
	default:
		return false
	}
	item := &Job{ID: id, Kind: "wordpress.restore", State: "running", User: user, StartedAt: time.Now().UTC()}
	j.mu.Lock()
	j.items[id] = item
	j.mu.Unlock()
	go func() {
		defer func() { <-j.slots }()
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		defer j.mu.Unlock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.WPress = &result
		}
	}()
	return true
}

type Jobs struct {
	mu            sync.RWMutex
	items         map[string]*Job
	slots         chan struct{}
	activeDomains map[string]bool
}

func NewJobs() *Jobs {
	return &Jobs{items: make(map[string]*Job), slots: make(chan struct{}, 2), activeDomains: make(map[string]bool)}
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
func (j *Jobs) Submit(id, user string, work func() (ImportResult, error)) bool {
	select {
	case j.slots <- struct{}{}:
	default:
		return false
	}
	item := &Job{ID: id, Kind: "cpmove.restore", State: "running", User: user, StartedAt: time.Now().UTC()}
	j.mu.Lock()
	j.items[id] = item
	j.mu.Unlock()
	go func() {
		defer func() { <-j.slots }()
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		defer j.mu.Unlock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Result = &result
		}
	}()
	return true
}

func (j *Jobs) SubmitCertificate(id, domain string, work func() (CertificateResult, error)) bool {
	select {
	case j.slots <- struct{}{}:
	default:
		return false
	}
	j.mu.Lock()
	if j.activeDomains[domain] {
		j.mu.Unlock()
		<-j.slots
		return false
	}
	j.activeDomains[domain] = true
	j.mu.Unlock()
	item := &Job{ID: id, Kind: "certificate.issue", State: "running", User: domain, StartedAt: time.Now().UTC()}
	j.mu.Lock()
	j.items[id] = item
	j.mu.Unlock()
	go func() {
		defer func() { j.mu.Lock(); delete(j.activeDomains, domain); j.mu.Unlock(); <-j.slots }()
		result, err := work()
		now := time.Now().UTC()
		j.mu.Lock()
		defer j.mu.Unlock()
		item.FinishedAt = &now
		if err != nil {
			item.State = "failed"
			item.Error = err.Error()
		} else {
			item.State = "completed"
			item.Certificate = &result
		}
	}()
	return true
}
func (j *Jobs) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	j.mu.Lock()
	defer j.mu.Unlock()
	for id, item := range j.items {
		if item.FinishedAt != nil && item.FinishedAt.Before(cutoff) {
			delete(j.items, id)
		}
	}
}
