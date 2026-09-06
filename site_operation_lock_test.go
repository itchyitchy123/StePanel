package main

import (
	"testing"
	"time"
)

func TestSiteOperationLocksSerializeSameSite(t *testing.T) {
	var locks siteOperationLocks
	releaseFirst := locks.acquire("demo")
	started := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		release := locks.acquire("demo")
		close(started)
		release()
		close(finished)
	}()
	select {
	case <-started:
		t.Fatal("same-site operation acquired its lock concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	releaseFirst()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("same-site operation did not acquire after release")
	}
	if len(locks.locks) != 0 {
		t.Fatalf("lock entries leaked: %d", len(locks.locks))
	}
}

func TestSiteOperationLocksAllowDifferentSites(t *testing.T) {
	var locks siteOperationLocks
	first := locks.acquire("one")
	acquired := make(chan struct{})
	go func() {
		release := locks.acquire("two")
		close(acquired)
		release()
	}()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("different-site operation was blocked")
	}
	first()
}

func TestSiteOperationLocksAcquireManyHasStableOrder(t *testing.T) {
	var locks siteOperationLocks
	first := locks.acquireMany("site-b", "site-a")
	finished := make(chan struct{})
	go func() {
		second := locks.acquireMany("site-a", "site-b")
		second()
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("related operation acquired locks before the first operation released them")
	case <-time.After(20 * time.Millisecond):
	}
	first()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("related operation did not acquire after release")
	}
	if len(locks.locks) != 0 {
		t.Fatalf("lock entries leaked after multi-lock operation: %d", len(locks.locks))
	}
}
