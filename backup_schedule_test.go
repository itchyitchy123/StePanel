package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupScheduleLifecycle(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web", "sites", "demo", "public")
	if err := os.MkdirAll(web, 0750); err != nil {
		t.Fatal(err)
	}
	store, err := openBackupSchedules(filepath.Join(root, "schedules.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Config: Config{WebRoot: filepath.Join(root, "web")}, Auth: Auth{}, Schedules: store}
	req := httptest.NewRequest(http.MethodPut, "/api/backup-schedules", strings.NewReader(`{"site":"demo","interval_minutes":60}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	a.backupSchedules(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"enabled":true`) {
		t.Fatalf("save: %d %s", res.Code, res.Body.String())
	}
	list := httptest.NewRecorder()
	a.backupSchedules(list, httptest.NewRequest(http.MethodGet, "/api/backup-schedules", nil))
	if !strings.Contains(list.Body.String(), `"keep_last":7`) {
		t.Fatalf("default retention missing from response: %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"site":"demo"`) {
		t.Fatalf("list: %s", list.Body.String())
	}
}
