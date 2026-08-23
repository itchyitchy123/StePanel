package main

import (
	"sync"
	"time"
)

type Job struct {
	ID         string        `json:"id"`
	Kind       string        `json:"kind"`
	State      string        `json:"state"`
	User       string        `json:"user"`
	Result     *ImportResult `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
}
type Jobs struct {
	mu    sync.RWMutex
	items map[string]*Job
}

func NewJobs() *Jobs { return &Jobs{items: make(map[string]*Job)} }
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
func (j *Jobs) Submit(id, user string, work func() (ImportResult, error)) {
	item := &Job{ID: id, Kind: "cpmove.restore", State: "running", User: user, StartedAt: time.Now().UTC()}
	j.mu.Lock()
	j.items[id] = item
	j.mu.Unlock()
	go func() {
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
