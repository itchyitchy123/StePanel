// Package jobs contains concurrency primitives used by asynchronous
// control-plane work. It deliberately has no HTTP or platform dependencies.
package jobs

// Admission is a bounded semaphore for long-running jobs.
type Admission struct{ slots chan struct{} }

// NewAdmission creates a bounded admission controller. Non-positive limits
// are normalized to one so callers cannot accidentally disable admission.
func NewAdmission(limit int) *Admission {
	if limit < 1 {
		limit = 1
	}
	return &Admission{slots: make(chan struct{}, limit)}
}

// Acquire claims one slot without waiting.
func (a *Admission) Acquire() bool {
	select {
	case a.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release returns a slot previously acquired by Acquire.
func (a *Admission) Release() { <-a.slots }
