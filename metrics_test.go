package main

import (
	"strings"
	"testing"
)

func TestMetricsWrite(t *testing.T) {
	m := NewMetrics()
	m.RestoreStarted()
	m.RestoreFinished(nil)
	m.RestoreStarted()
	m.RestoreFinished(assertionError{})

	var output strings.Builder
	m.Write(&output)
	for _, want := range []string{
		"stepanel_up 1",
		"stepanel_restore_jobs_started_total 2",
		"stepanel_restore_jobs_completed_total 1",
		"stepanel_restore_jobs_failed_total 1",
		"stepanel_restore_jobs_active 0",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("metrics output missing %q:\n%s", want, output.String())
		}
	}
}

type assertionError struct{}

func (assertionError) Error() string { return "test failure" }
