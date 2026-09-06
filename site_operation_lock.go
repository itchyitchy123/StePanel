package main

import (
	"sort"
	"sync"
)

// siteOperationLocks serializes operations that share a site-level staging,
// environment, database, route, or release path while allowing unrelated sites
// to proceed in parallel. References keep a lock alive for waiters without
// leaking entries.
type siteOperationLocks struct {
	mu    sync.Mutex
	locks map[string]*siteOperationLock
}

type siteOperationLock struct {
	mu   sync.Mutex
	refs int
}

func (l *siteOperationLocks) acquire(site string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*siteOperationLock)
	}
	lock := l.locks[site]
	if lock == nil {
		lock = &siteOperationLock{}
		l.locks[site] = lock
	}
	lock.refs++
	l.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		l.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(l.locks, site)
		}
		l.mu.Unlock()
	}
}

// acquireMany obtains a stable set of related operation locks in lexical
// order. This lets a mutation coordinate both a site and a generated object
// identity (for example, a route filename) without introducing lock-order
// deadlocks between concurrent requests.
func (l *siteOperationLocks) acquireMany(keys ...string) func() {
	unique := make(map[string]struct{}, len(keys))
	ordered := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	unlockers := make([]func(), 0, len(ordered))
	for _, key := range ordered {
		unlockers = append(unlockers, l.acquire(key))
	}
	return func() {
		for i := len(unlockers) - 1; i >= 0; i-- {
			unlockers[i]()
		}
	}
}
