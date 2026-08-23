package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalBackendValidation(t *testing.T) {
	valid := []string{"http://127.0.0.1:3000", "http://localhost:8080", "http://192.168.1.20:9000"}
	for _, value := range valid {
		if _, err := localBackend(value); err != nil {
			t.Errorf("localBackend(%q) failed: %v", value, err)
		}
	}
	invalid := []string{"https://127.0.0.1:3000", "http://example.com:3000", "http://127.0.0.1", "http://127.0.0.1:3000/admin", "http://user:pass@127.0.0.1:3000"}
	for _, value := range invalid {
		if _, err := localBackend(value); err == nil {
			t.Errorf("localBackend(%q) unexpectedly passed", value)
		}
	}
}

func TestProxyDeleteRestoresConfigWhenReloadFails(t *testing.T) {
	root := t.TempDir()
	proxyRoot := filepath.Join(root, "proxy")
	if err := os.MkdirAll(proxyRoot, 0750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(proxyRoot, "account-example_com.conf")
	original := []byte("original config\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	reloader := filepath.Join(root, "reload-fails")
	if err := os.WriteFile(reloader, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	app := &App{Config: Config{ProxyRoot: proxyRoot, ApacheReload: reloader}, Auth: Auth{}}
	req := httptest.NewRequest(http.MethodDelete, "/api/proxy/account-example_com.conf", nil)
	response := httptest.NewRecorder()
	app.proxyManage(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	restored, err := os.ReadFile(path)
	if err != nil || string(restored) != string(original) {
		t.Fatalf("proxy config was not restored: %q, %v", restored, err)
	}
}
