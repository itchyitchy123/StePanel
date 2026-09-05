package main

import (
	"strings"
	"testing"
	"time"
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
		"stepanel_http_request_duration_seconds_bucket{le=\"+Inf\"}",
		"stepanel_http_request_duration_seconds_count 0",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("metrics output missing %q:\n%s", want, output.String())
		}
	}
}

func TestMetricsDurationHistogramIsCumulativeAndNonNegative(t *testing.T) {
	m := NewMetrics()
	m.ObserveHTTP(200, 20*time.Millisecond)
	m.ObserveHTTP(500, -time.Second)
	var output strings.Builder
	m.Write(&output)
	if !strings.Contains(output.String(), "stepanel_http_request_duration_seconds_count 2") {
		t.Fatalf("histogram count missing: %s", output.String())
	}
	if !strings.Contains(output.String(), "stepanel_http_request_duration_seconds_sum 0.020000") {
		t.Fatalf("negative duration was not clamped: %s", output.String())
	}
}

type assertionError struct{}

func (assertionError) Error() string { return "test failure" }
