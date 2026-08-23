package main

import (
	"fmt"
	"io"
	"sync/atomic"
)

// Metrics contains the small, dependency-free Prometheus surface exposed by
// StePanel. Keeping the counters in-process makes the control plane observable
// even in minimal installations without a metrics SDK or sidecar.
type Metrics struct {
	restoresStarted   atomic.Uint64
	restoresCompleted atomic.Uint64
	restoresFailed    atomic.Uint64
	activeRestores    atomic.Int64
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
}
