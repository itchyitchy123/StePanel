package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testTOTPSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestAccountStorePersistsOnlyValidatedAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := OpenAccountStore(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", []string{"site-one"})
	if err != nil {
		t.Fatal(err)
	}
	if account.PasswordHash != "" || account.TOTPSecret != "" {
		t.Fatal("account response exposed authentication secrets")
	}
	if !store.OwnsSite("customer", "site-one") || store.OwnsSite("customer", "site-two") {
		t.Fatal("site assignment was not enforced")
	}
	if _, err := store.Create("over-limit", "a sufficiently long customer password", testTOTPSecret, "starter", []string{"one", "two"}); err == nil {
		t.Fatal("starter plan accepted too many sites")
	}
	reopened, err := OpenAccountStore(path)
	if err != nil || !reopened.OwnsSite("customer", "site-one") {
		t.Fatal("persisted account state did not reopen cleanly")
	}
}

func TestCustomerLoginRequiresAccountTOTP(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	t.Setenv("STEPANEL_ADMIN_TOTP_SECRET", "")
	store, err := OpenAccountStore(t.TempDir() + "/accounts.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", []string{"site-one"}); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	auth.Accounts = store
	secret, err := decodeTOTPSecret(testTOTPSecret)
	if err != nil {
		t.Fatal(err)
	}
	code := totpCode(secret, uint64(time.Now().Unix()/30))
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=customer&password=a+sufficiently+long+customer+password&totp="+code))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	auth.Login(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("customer login status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range response.Result().Cookies() {
		sessionRequest.AddCookie(cookie)
	}
	if !auth.validSession(sessionRequest) || auth.IsAdministrator(sessionRequest) || auth.UsernameForRequest(sessionRequest) != "customer" {
		t.Fatal("customer session was not scoped to the customer identity")
	}
}
