package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteDeployUsesValidatedVHostHelper(t *testing.T) {
	root := t.TempDir()
	webRoot := filepath.Join(root, "www")
	if err := os.MkdirAll(filepath.Join(webRoot, "sites", "account", "public"), 0750); err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(root, "args")
	helper := filepath.Join(root, "vhostctl")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$TEST_VHOST_ARGS\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_VHOST_ARGS", argsPath)
	app := &App{Config: Config{WebRoot: webRoot, VHostRoot: filepath.Join(root, "vhosts"), VHostCtl: helper}}
	req := httptest.NewRequest(http.MethodPost, "/api/sites/deploy", bytes.NewBufferString(`{"site":"account","domain":"WWW.Example.com"}`))
	response := httptest.NewRecorder()
	app.siteDeploy(response, req)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertTestFile(t, argsPath, "apply account www.example.com\n")
}

func TestSiteDeployRejectsMissingDocumentRoot(t *testing.T) {
	app := &App{Config: Config{WebRoot: t.TempDir(), VHostCtl: "/not-run"}}
	req := httptest.NewRequest(http.MethodPost, "/api/sites/deploy", bytes.NewBufferString(`{"site":"missing","domain":"example.com"}`))
	response := httptest.NewRecorder()
	app.siteDeploy(response, req)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "document root") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSiteListFiltersUnmanagedFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "site-account-example_com.conf"), "managed")
	writeTestFile(t, filepath.Join(root, "unmanaged.conf"), "unmanaged")
	app := &App{Config: Config{VHostRoot: root}}
	response := httptest.NewRecorder()
	app.siteList(response, httptest.NewRequest(http.MethodGet, "/api/sites", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "site-account-example_com.conf") || strings.Contains(response.Body.String(), "unmanaged.conf") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
