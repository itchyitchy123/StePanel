package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthRejectsInvalidBearerInsteadOfFallingBackToCookie(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	auth.apiTokens = &apiTokenStore{}
	request := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	response := httptest.NewRecorder()
	auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("handler accepted invalid bearer") })).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAPITokenLifecycleStoresOnlyHashAndRevokes(t *testing.T) {
	db, err := openControlPlaneDB(filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &apiTokenStore{db: db}
	expires := time.Now().Add(time.Hour).Unix()
	item, secret, err := store.create("customer", "automation", &expires)
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || item.Prefix == "" {
		t.Fatal("token was not issued")
	}
	var stored string
	if err := db.QueryRow(`SELECT token_hash FROM api_tokens WHERE id = ?`, item.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == secret {
		t.Fatal("plaintext token was persisted")
	}
	if username, ok := store.authenticate(secret); !ok || username != "customer" {
		t.Fatalf("authenticate = %q, %v", username, ok)
	}
	if err := store.revoke("customer", item.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.authenticate(secret); ok {
		t.Fatal("revoked token remained valid")
	}
}

func TestAdministratorAPITokenScopes(t *testing.T) {
	db, err := openControlPlaneDB(filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &apiTokenStore{db: db}
	readItem, readSecret, err := store.createScoped("admin", "read-only", nil, []string{"admin:read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(readItem.Scopes) != 1 || readItem.Scopes[0] != "admin:read" {
		t.Fatalf("read token scopes = %#v", readItem.Scopes)
	}
	username, scopes, ok := store.authenticateWithScopes(readSecret)
	if !ok || username != "admin" || len(scopes) != 1 || scopes[0] != "admin:read" {
		t.Fatalf("authenticated scopes = %q %#v %v", username, scopes, ok)
	}

	t.Setenv("STEPANEL_ADMIN_USERNAME", "admin")
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	auth.apiTokens = store
	readRequest := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	readRequest.Header.Set("Authorization", "Bearer "+readSecret)
	readResponse := httptest.NewRecorder()
	auth.RequireAdministrator(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(readResponse, readRequest)
	if readResponse.Code != http.StatusNoContent {
		t.Fatalf("read token GET status = %d", readResponse.Code)
	}
	mutateRequest := httptest.NewRequest(http.MethodPost, "/api/cloud/action", nil)
	mutateRequest.Header.Set("Authorization", "Bearer "+readSecret)
	mutateResponse := httptest.NewRecorder()
	auth.RequireAdministrator(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(mutateResponse, mutateRequest)
	if mutateResponse.Code != http.StatusForbidden {
		t.Fatalf("read token POST status = %d", mutateResponse.Code)
	}

	_, operateSecret, err := store.createScoped("admin", "operator", nil, []string{"admin:operate"})
	if err != nil {
		t.Fatal(err)
	}
	operateRequest := httptest.NewRequest(http.MethodPost, "/api/cloud/action", nil)
	operateRequest.Header.Set("Authorization", "Bearer "+operateSecret)
	operateResponse := httptest.NewRecorder()
	auth.RequireAdministrator(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(operateResponse, operateRequest)
	if operateResponse.Code != http.StatusNoContent {
		t.Fatalf("operate token POST status = %d", operateResponse.Code)
	}
}
