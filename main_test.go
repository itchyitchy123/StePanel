package main

import (
	"bytes"
	"errors"
	"html/template"
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

func TestEmbeddedDashboardTemplate(t *testing.T) {
	data, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	view, err := template.New("index.html").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(string(data))
	if err != nil {
		t.Fatalf("parse embedded dashboard: %v", err)
	}
	var rendered bytes.Buffer
	if err := view.Execute(&rendered, map[string]any{"Title": "StePanel", "Now": time.Now(), "Servers": []ServiceSummary{{Name: "apache2", Status: "active"}}, "Healthy": 1, "Alerts": 0, "Security": []SecurityCheck{}, "Jobs": []Job{}, "Capabilities": map[string]bool{}}); err != nil {
		t.Fatalf("render embedded dashboard: %v", err)
	}
	if strings.Contains(rendered.String(), "Stephan") || !strings.Contains(rendered.String(), "Infrastructure overview") {
		t.Fatal("dashboard contains simulated content or is missing its live overview")
	}
}

func TestLoggingAddsBrowserSecurityHeaders(t *testing.T) {
	handler := logging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), nil, false)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID header is missing")
	}
}

func TestAllowMethodsRejectsUnexpectedMethod(t *testing.T) {
	called := false
	handler := allowMethods(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), http.MethodGet)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/health", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || called {
		t.Fatalf("status = %d, Allow = %q, called = %v", response.Code, response.Header().Get("Allow"), called)
	}
}

func TestNormalizeAPIErrors(t *testing.T) {
	handler := normalizeAPIErrors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid request", http.StatusBadRequest)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/example", nil))
	if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/json" || !strings.Contains(response.Body.String(), `"error":"invalid request"`) {
		t.Fatalf("unexpected normalized error: %d %s", response.Code, response.Body.String())
	}
}

func TestDecodeJSONRejectsUnknownAndTrailingValues(t *testing.T) {
	for _, body := range []string{`{"name":"ok","unknown":true}`, `{"name":"ok"}{"name":"again"}`} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		response := httptest.NewRecorder()
		var input struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(response, request, 1024, &input); err == nil {
			t.Fatalf("decodeJSON accepted %q", body)
		}
	}
}

func TestAuthenticatedOperationalEndpoints(t *testing.T) {
	root := t.TempDir()
	app := &App{Config: Config{ImportRoot: root, WebRoot: root}, Auth: Auth{}, Jobs: NewJobs(), Metrics: NewMetrics()}
	server := http.NewServeMux()
	server.HandleFunc("/api/services", app.services)
	server.HandleFunc("/api/ftp", app.ftpStatus)
	server.HandleFunc("/api/security/audit", app.securityAudit)
	server.HandleFunc("/api/doctor", app.doctor)
	server.HandleFunc("/api/cloud", app.cloudInventory)

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
	doctor := httptest.NewRecorder()
	server.ServeHTTP(doctor, httptest.NewRequest(http.MethodGet, "/api/doctor", nil))
	if doctor.Code != http.StatusOK || !strings.Contains(doctor.Body.String(), `"healthy"`) || !strings.Contains(doctor.Body.String(), `"web-root-disk"`) {
		t.Fatalf("unexpected doctor response: %d %s", doctor.Code, doctor.Body.String())
	}
	cloud := httptest.NewRecorder()
	server.ServeHTTP(cloud, httptest.NewRequest(http.MethodGet, "/api/cloud", nil))
	if cloud.Code != http.StatusOK || !strings.Contains(cloud.Body.String(), "no cloud provider configured") {
		t.Fatalf("unexpected cloud response: %d %s", cloud.Code, cloud.Body.String())
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

func TestJobsListNewestFirst(t *testing.T) {
	jobs := NewJobs()
	jobs.items["old"] = &Job{ID: "old", StartedAt: time.Unix(1, 0)}
	jobs.items["new"] = &Job{ID: "new", StartedAt: time.Unix(2, 0)}
	items := jobs.List(1)
	if len(items) != 1 || items[0].ID != "new" {
		t.Fatalf("jobs = %#v, want newest job", items)
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

func TestJobsEnforceConfiguredGlobalCapacity(t *testing.T) {
	jobs := newJobs("", 1)
	started := make(chan struct{})
	release := make(chan struct{})
	if err := jobs.Submit("job-1", "site-one", func() (ImportResult, error) {
		close(started)
		<-release
		return ImportResult{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := jobs.Submit("job-2", "site-two", func() (ImportResult, error) { return ImportResult{}, nil }); !errors.Is(err, ErrJobBusy) {
		t.Fatalf("capacity error = %v, want ErrJobBusy", err)
	}
	close(release)
}
