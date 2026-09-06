package main

import "sync"

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
