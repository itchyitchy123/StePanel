package operations

import (
	"testing"
	"time"
)

func TestLocksSerializeSameKey(t *testing.T) {
	var locks Locks
	releaseFirst := locks.Acquire("demo")
	started := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		release := locks.Acquire("demo")
		close(started)
		release()
		close(finished)
	}()
	select {
	case <-started:
		t.Fatal("same-key operation acquired concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	releaseFirst()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("same-key operation did not acquire after release")
	}
	if locks.Len() != 0 {
		t.Fatalf("lock entries leaked: %d", locks.Len())
	}
}

func TestLocksAcquireManyUsesStableOrder(t *testing.T) {
	var locks Locks
	first := locks.AcquireMany("site-b", "site-a")
	finished := make(chan struct{})
	go func() {
		second := locks.AcquireMany("site-a", "site-b")
		second()
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("related operation acquired before release")
	case <-time.After(20 * time.Millisecond):
	}
	first()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("related operation did not acquire after release")
	}
}
