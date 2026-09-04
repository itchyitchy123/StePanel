package main

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// Metrics contains the small, dependency-free Prometheus surface exposed by
// StePanel. Keeping the counters in-process makes the control plane observable
// even in minimal installations without a metrics SDK or sidecar.
type Metrics struct {
	restoresStarted   atomic.Uint64
	restoresCompleted atomic.Uint64
	restoresFailed    atomic.Uint64
	activeRestores    atomic.Int64
	httpRequests      atomic.Uint64
	httpErrors        atomic.Uint64
	httpDurationNanos atomic.Uint64
	httpStatus        [6]atomic.Uint64
}

func (m *Metrics) ObserveHTTP(status int, duration time.Duration) {
	m.httpRequests.Add(1)
	if status >= 500 {
		m.httpErrors.Add(1)
	}
	m.httpDurationNanos.Add(uint64(duration))
	bucket := status / 100
	if bucket >= 1 && bucket <= 5 {
		m.httpStatus[bucket].Add(1)
	}
}

func NewMetrics() *Metrics { return &Metrics{} }

func (m *Metrics) RestoreStarted() { m.restoresStarted.Add(1); m.activeRestores.Add(1) }

func (m *Metrics) RestoreFinished(err error) {
	m.activeRestores.Add(-1)
	if err != nil {
		m.restoresFailed.Add(1)
		return
	}
	m.restoresCompleted.Add(1)
}

func (m *Metrics) Write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "# HELP stepanel_up StePanel process health\n# TYPE stepanel_up gauge\nstepanel_up 1\n")
	_, _ = fmt.Fprintf(w, "# HELP stepanel_restore_jobs_started_total Restore jobs accepted\n# TYPE stepanel_restore_jobs_started_total counter\nstepanel_restore_jobs_started_total %d\n", m.restoresStarted.Load())
	_, _ = fmt.Fprintf(w, "# HELP stepanel_restore_jobs_completed_total Restore jobs completed successfully\n# TYPE stepanel_restore_jobs_completed_total counter\nstepanel_restore_jobs_completed_total %d\n", m.restoresCompleted.Load())
	_, _ = fmt.Fprintf(w, "# HELP stepanel_restore_jobs_failed_total Restore jobs that failed\n# TYPE stepanel_restore_jobs_failed_total counter\nstepanel_restore_jobs_failed_total %d\n", m.restoresFailed.Load())
	_, _ = fmt.Fprintf(w, "# HELP stepanel_restore_jobs_active Current restore jobs\n# TYPE stepanel_restore_jobs_active gauge\nstepanel_restore_jobs_active %d\n", m.activeRestores.Load())
	_, _ = fmt.Fprintf(w, "# HELP stepanel_http_requests_total HTTP requests served\n# TYPE stepanel_http_requests_total counter\nstepanel_http_requests_total %d\n", m.httpRequests.Load())
	_, _ = fmt.Fprintf(w, "# HELP stepanel_http_errors_total HTTP 5xx responses\n# TYPE stepanel_http_errors_total counter\nstepanel_http_errors_total %d\n", m.httpErrors.Load())
	_, _ = fmt.Fprintf(w, "# HELP stepanel_http_request_duration_seconds_total Cumulative HTTP request duration\n# TYPE stepanel_http_request_duration_seconds_total counter\nstepanel_http_request_duration_seconds_total %.6f\n", float64(m.httpDurationNanos.Load())/float64(time.Second))
	_, _ = fmt.Fprintf(w, "# HELP stepanel_http_responses_total HTTP responses by status class\n# TYPE stepanel_http_responses_total counter\n")
	for bucket := 1; bucket <= 5; bucket++ {
		_, _ = fmt.Fprintf(w, "stepanel_http_responses_total{class=\"%dxx\"} %d\n", bucket, m.httpStatus[bucket].Load())
	}
}
