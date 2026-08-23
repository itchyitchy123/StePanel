package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthAndMetricsEndpoints(t *testing.T) {
	app := &App{Config: Config{}, Auth: Auth{}, Jobs: NewJobs(), Metrics: NewMetrics()}
	app.Metrics.RestoreStarted()
	app.Metrics.RestoreFinished(nil)

	server := http.NewServeMux()
	server.HandleFunc("/api/health", app.health)
	server.HandleFunc("/metrics", app.metrics)

	health := httptest.NewRecorder()
	server.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"ok":true`) {
		t.Fatalf("unexpected health response: %d %s", health.Code, health.Body.String())
	}
	metrics := httptest.NewRecorder()
	server.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), "stepanel_restore_jobs_completed_total 1") {
		t.Fatalf("unexpected metrics response: %d %s", metrics.Code, metrics.Body.String())
	}
}

func TestAuthRequireProtectsAPI(t *testing.T) {
	auth := Auth{Enabled: true, Username: "admin", Secret: "12345678901234567890123456789012"}
	handler := auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestJobsCompleteAndCleanup(t *testing.T) {
	jobs := NewJobs()
	done := make(chan struct{})
	jobs.Submit("job-1", "site", func() (ImportResult, error) {
		close(done)
		return ImportResult{}, nil
	})
	<-done
	var job Job
	for i := 0; i < 20; i++ {
		var ok bool
		job, ok = jobs.Get("job-1")
		if ok && job.State == "completed" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if job.State != "completed" {
		t.Fatalf("job state = %q, want completed", job.State)
	}
	if _, ok := jobs.Get("missing"); ok {
		t.Fatal("missing job unexpectedly found")
	}
}
