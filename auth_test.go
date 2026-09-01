package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestAuthRequiresSessionSecret(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "")
	if _, err := NewAuth(true); err == nil {
		t.Fatal("expected missing session secret to fail")
	}
}

func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("hosting-grade-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("hosting-grade-password")); err != nil {
		t.Fatalf("generated hash did not verify: %v", err)
	}
}

func TestAuthRejectsOversizedLogin(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	body := "username=admin&password=" + strings.Repeat("x", 65<<10)
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	auth.Login(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("login status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestAuthSessionAndCSRF(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/login", strings.NewReader("username=admin&password=correct+horse+battery+staple"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	auth.Login(response, request)
	if response.Code != 303 {
		t.Fatalf("login status = %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatal("expected session and csrf cookies")
	}
	sessionRequest := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range cookies {
		sessionRequest.AddCookie(cookie)
	}
	if !auth.validSession(sessionRequest) {
		t.Fatal("expected valid session")
	}
	csrfRequest := httptest.NewRequest("POST", "/api/change", strings.NewReader("csrf="+cookies[1].Value))
	csrfRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		csrfRequest.AddCookie(cookie)
	}
	if !auth.CSRF(csrfRequest) {
		t.Fatal("expected valid csrf token")
	}
}

func TestTOTPValidationRejectsReplay(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	t.Setenv("STEPANEL_ADMIN_TOTP_SECRET", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1234567890, 0)
	code := totpCode([]byte("12345678901234567890"), uint64(now.Unix()/30))
	if !auth.consumeTOTP(code, now) {
		t.Fatal("valid TOTP was rejected")
	}
	if auth.consumeTOTP(code, now) {
		t.Fatal("replayed TOTP was accepted")
	}
}

func TestAuthRejectsInvalidTOTPSecret(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	t.Setenv("STEPANEL_ADMIN_TOTP_SECRET", "not-base32")
	if _, err := NewAuth(true); err == nil {
		t.Fatal("invalid TOTP secret was accepted")
	}
}

func TestAuthenticatedMutationFailsClosedWhenAuditIsUnavailable(t *testing.T) {
	t.Cleanup(func() {
		auditMu.Lock()
		auditPersistenceErr = nil
		auditMu.Unlock()
	})
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	t.Setenv("STEPANEL_ADMIN_TOTP_SECRET", "")
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	auth.AuditLog = filepath.Join(blocked, "audit.jsonl")
	expiry := time.Now().Add(time.Hour).Unix()
	payload := auth.Username + "|" + strconv.FormatInt(expiry, 10)
	token := payload + "|" + auth.sign(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/change", nil)
	request.AddCookie(&http.Cookie{Name: "stepanel_session", Value: base64.RawURLEncoding.EncodeToString([]byte(token))})
	called := false
	handler := auth.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || called {
		t.Fatalf("status = %d, handler called = %v", response.Code, called)
	}
}
