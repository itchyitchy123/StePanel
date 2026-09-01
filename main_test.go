package main

import (
	"errors"
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

func TestLoggingAddsBrowserSecurityHeaders(t *testing.T) {
	handler := logging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestAuthenticatedOperationalEndpoints(t *testing.T) {
	root := t.TempDir()
	app := &App{Config: Config{ImportRoot: root, WebRoot: root}, Auth: Auth{}, Jobs: NewJobs(), Metrics: NewMetrics()}
	server := http.NewServeMux()
	server.HandleFunc("/api/services", app.services)
	server.HandleFunc("/api/ftp", app.ftpStatus)
	server.HandleFunc("/api/security/audit", app.securityAudit)

	services := httptest.NewRecorder()
	server.ServeHTTP(services, httptest.NewRequest(http.MethodGet, "/api/services", nil))
	if services.Code != http.StatusOK || !strings.Contains(services.Body.String(), `"services"`) || !strings.Contains(services.Body.String(), `"vsftpd"`) {
		t.Fatalf("unexpected services response: %d %s", services.Code, services.Body.String())
	}
	ftp := httptest.NewRecorder()
	server.ServeHTTP(ftp, httptest.NewRequest(http.MethodGet, "/api/ftp", nil))
	if ftp.Code != http.StatusOK || !strings.Contains(ftp.Body.String(), `"local_user_chroot":true`) {
		t.Fatalf("unexpected FTP response: %d %s", ftp.Code, ftp.Body.String())
	}
	audit := httptest.NewRecorder()
	server.ServeHTTP(audit, httptest.NewRequest(http.MethodGet, "/api/security/audit", nil))
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), `"checks"`) {
		t.Fatalf("unexpected audit response: %d %s", audit.Code, audit.Body.String())
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
	if err := jobs.Submit("job-1", "site", func() (ImportResult, error) {
		close(done)
		return ImportResult{}, nil
	}); err != nil {
		t.Fatal(err)
	}
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

func TestJobsRejectConcurrentRestoresForSameSite(t *testing.T) {
	jobs := NewJobs()
	started := make(chan struct{})
	release := make(chan struct{})
	if err := jobs.Submit("job-1", "site", func() (ImportResult, error) {
		close(started)
		<-release
		return ImportResult{}, nil
	}); err != nil {
		t.Fatalf("first restore was rejected: %v", err)
	}
	<-started
	if err := jobs.SubmitWPress("job-2", "site", func() (WPressResult, error) {
		return WPressResult{}, nil
	}); !errors.Is(err, ErrJobBusy) {
		t.Fatalf("concurrent restore error = %v, want ErrJobBusy", err)
	}
	close(release)
}
