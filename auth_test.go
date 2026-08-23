package main

import (
    "net/http/httptest"
    "strings"
    "testing"
)

func TestAuthRequiresSessionSecret(t *testing.T) {
    t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
    t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
    t.Setenv("STEPANEL_SESSION_SECRET", "")
    if _, err := NewAuth(true); err == nil { t.Fatal("expected missing session secret to fail") }
}

func TestAuthSessionAndCSRF(t *testing.T) {
    t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
    t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
    t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
    auth, err := NewAuth(true); if err != nil { t.Fatal(err) }
    request := httptest.NewRequest("POST", "/login", strings.NewReader("username=admin&password=correct+horse+battery+staple")); request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    response := httptest.NewRecorder(); auth.Login(response, request)
    if response.Code != 303 { t.Fatalf("login status = %d", response.Code) }
    cookies := response.Result().Cookies(); if len(cookies) < 2 { t.Fatal("expected session and csrf cookies") }
    sessionRequest := httptest.NewRequest("GET", "/", nil); for _, cookie := range cookies { sessionRequest.AddCookie(cookie) }; if !auth.validSession(sessionRequest) { t.Fatal("expected valid session") }
    csrfRequest := httptest.NewRequest("POST", "/api/change", strings.NewReader("csrf="+cookies[1].Value)); csrfRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded"); for _, cookie := range cookies { csrfRequest.AddCookie(cookie) }; if !auth.CSRF(csrfRequest) { t.Fatal("expected valid csrf token") }
}
