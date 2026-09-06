package main

import (
	"net/http"
	"net/http/httptest"
	"os"
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
	if _, err := store.Create("other", "another sufficiently long customer password", testTOTPSecret, "starter", []string{"site-one"}); err == nil || !strings.Contains(err.Error(), "already assigned") {
		t.Fatalf("duplicate site assignment error = %v", err)
	}
	reopened, err := OpenAccountStore(path)
	if err != nil || !reopened.OwnsSite("customer", "site-one") {
		t.Fatal("persisted account state did not reopen cleanly")
	}
}

func TestAccountStoreUpdatesPlanAndAssignmentsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := OpenAccountStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", []string{"site-one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("other", "another sufficiently long customer password", testTOTPSecret, "starter", []string{"site-two"}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update("customer", "professional", []string{"site-one"})
	if err != nil || updated.Plan != "professional" || len(updated.Sites) != 1 {
		t.Fatalf("account update = %#v, err = %v", updated, err)
	}
	if _, err := store.Update("customer", "professional", []string{"site-two"}); err == nil || !strings.Contains(err.Error(), "already assigned") {
		t.Fatalf("expected duplicate ownership rejection, got %v", err)
	}
	account, ok := store.Get("customer")
	if !ok || account.Plan != "professional" || len(account.Sites) != 1 || account.Sites[0] != "site-one" {
		t.Fatalf("failed update changed persisted account: %#v", account)
	}
}

func TestAccountStoreRejectsDuplicateSiteOwnershipOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	data := `[
  {"username":"one","password_hash":"$2a$10$7EqJtq98hPqEX7fNZaFWoO5Z8k4Q4kXQ9rXv4bQfQv7xFfZpZq7uK","totp_secret":"GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ","plan":"starter","sites":["shared"],"created_at":"2026-01-01T00:00:00Z"},
  {"username":"two","password_hash":"$2a$10$7EqJtq98hPqEX7fNZaFWoO5Z8k4Q4kXQ9rXv4bQfQv7xFfZpZq7uK","totp_secret":"GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ","plan":"starter","sites":["shared"],"created_at":"2026-01-01T00:00:00Z"}
]`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAccountStore(path); err == nil || !strings.Contains(err.Error(), "assigned to both") {
		t.Fatalf("duplicate ownership load error = %v", err)
	}
}

func TestAccountStoreEncryptsTOTPAndSupportsRegeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := OpenAccountStore(path, "account-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testTOTPSecret) || !strings.Contains(string(data), "totp_encrypted") {
		t.Fatalf("account state did not encrypt TOTP secret: %s", data)
	}
	reopened, err := OpenAccountStore(path, "account-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	if account, ok := reopened.Get("customer"); !ok || account.TOTPSecret != testTOTPSecret {
		t.Fatal("encrypted TOTP secret did not decrypt")
	}
	account, replacement, err := reopened.ResetTOTP("customer")
	if err != nil || replacement == testTOTPSecret || account.TOTPSecret != "" {
		t.Fatalf("MFA regeneration = %#v, %q, %v", account, replacement, err)
	}
	if _, err := decodeTOTPSecret(replacement); err != nil {
		t.Fatalf("replacement TOTP was invalid: %v", err)
	}
}

func TestEncryptedAccountStateRequiresKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := OpenAccountStore(path, "account-encryption-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAccountStore(path); err == nil || !strings.Contains(err.Error(), "STEPANEL_ACCOUNT_KEY") {
		t.Fatalf("opening encrypted account state without key = %v", err)
	}
}

func TestRecoveryCodesAreOneTimeAndOnlyHashesPersist(t *testing.T) {
	store, err := OpenAccountStore(filepath.Join(t.TempDir(), "accounts.json"), "account-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", nil); err != nil {
		t.Fatal(err)
	}
	account, codes, err := store.GenerateRecoveryCodes("customer")
	if err != nil || len(codes) != 10 {
		t.Fatalf("GenerateRecoveryCodes = %d codes, %v", len(codes), err)
	}
	if len(account.RecoveryCodeHashes) != 0 {
		t.Fatal("recovery-code hashes were returned to the caller")
	}
	if listed := store.List(); len(listed) != 1 || len(listed[0].RecoveryCodeHashes) != 0 {
		t.Fatal("recovery-code hashes were exposed by account listing")
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), codes[0]) {
		t.Fatal("plaintext recovery code was persisted")
	}
	if ok, err := store.ConsumeRecoveryCode("customer", codes[0]); err != nil || !ok {
		t.Fatalf("first recovery-code use = %v, %v", ok, err)
	}
	if ok, err := store.ConsumeRecoveryCode("customer", codes[0]); err != nil || ok {
		t.Fatalf("replayed recovery code = %v, %v", ok, err)
	}
}

func TestCredentialRecoveryRotatesAllMFAMaterial(t *testing.T) {
	store, err := OpenAccountStore(filepath.Join(t.TempDir(), "accounts.json"), "account-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", nil); err != nil {
		t.Fatal(err)
	}
	account, temporaryPassword, totpSecret, codes, err := store.RecoverCredentials("customer")
	if err != nil || len(temporaryPassword) < 20 || len(codes) != 10 || account.TOTPSecret != "" {
		t.Fatalf("credential recovery = %#v, %q, %q, %d codes, %v", account, temporaryPassword, totpSecret, len(codes), err)
	}
	if !account.PasswordResetRequired || !account.MFAEnrollmentRequired {
		t.Fatal("credential recovery did not set completion requirements")
	}
	if _, err := decodeTOTPSecret(totpSecret); err != nil {
		t.Fatalf("recovered TOTP secret invalid: %v", err)
	}
	if updated, err := store.SetPassword("customer", "a new sufficiently long customer password"); err != nil || updated.PasswordResetRequired {
		t.Fatalf("password completion = %#v, %v", updated, err)
	}
	if updated, err := store.SetTOTP("customer", totpSecret); err != nil || updated.MFAEnrollmentRequired {
		t.Fatalf("MFA completion = %#v, %v", updated, err)
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
	if _, err := store.SetSuspended("customer", true); err != nil {
		t.Fatal(err)
	}
	if auth.validSession(sessionRequest) {
		t.Fatal("existing customer session survived suspension")
	}
}

func TestAccountLifecyclePersistsSuspensionAndLoginRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store, err := OpenAccountStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", []string{"site-one"}); err != nil {
		t.Fatal(err)
	}
	account, err := store.SetSuspended("customer", true)
	if err != nil || !account.Suspended {
		t.Fatalf("suspend = %#v, %v", account, err)
	}
	reopened, err := OpenAccountStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if account, ok := reopened.Get("customer"); !ok || !account.Suspended {
		t.Fatal("suspension was not persisted")
	}
	if err := reopened.RemoveLogin("customer"); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Get("customer"); ok {
		t.Fatal("removed customer login remained present")
	}
}

func TestSuspendedCustomerCannotLogin(t *testing.T) {
	t.Setenv("STEPANEL_ADMIN_PASSWORD", "correct horse battery staple")
	t.Setenv("STEPANEL_ADMIN_PASSWORD_HASH", "")
	t.Setenv("STEPANEL_SESSION_SECRET", "12345678901234567890123456789012")
	store, err := OpenAccountStore(t.TempDir() + "/accounts.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("customer", "a sufficiently long customer password", testTOTPSecret, "starter", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetSuspended("customer", true); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuth(true)
	if err != nil {
		t.Fatal(err)
	}
	auth.Accounts = store
	secret, _ := decodeTOTPSecret(testTOTPSecret)
	code := totpCode(secret, uint64(time.Now().Unix()/30))
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=customer&password=a+sufficiently+long+customer+password&totp="+code))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	auth.Login(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("suspended login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestSuspensionRevokesExistingCustomerSessions(t *testing.T) {
	expiry := time.Now().Add(time.Hour).Unix()
	registry := &sessionRegistry{entries: map[string]sessionEntry{"customer-session": {Username: "customer", Expiry: expiry}, "admin-session": {Username: "admin", Expiry: expiry}}}
	if err := registry.revokeUser("customer"); err != nil {
		t.Fatal(err)
	}
	if registry.valid("customer-session", "customer", expiry) {
		t.Fatal("customer session remained valid")
	}
	if _, ok := registry.entries["admin-session"]; !ok {
		t.Fatal("unrelated session was revoked")
	}
}

func TestSessionRevocationRestoresMemoryOnPersistenceFailure(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour).Unix()
	registry := &sessionRegistry{
		path: blocked + "/sessions.json",
		entries: map[string]sessionEntry{
			"customer-session": {Username: "customer", Expiry: expiry},
			"admin-session":    {Username: "admin", Expiry: expiry},
		},
	}
	if err := registry.revokeUser("customer"); err == nil {
		t.Fatal("expected session persistence failure")
	}
	if !registry.valid("customer-session", "customer", expiry) {
		t.Fatal("failed user revocation was not rolled back in memory")
	}
	if err := registry.revoke("customer-session"); err == nil {
		t.Fatal("expected single-session persistence failure")
	}
	if !registry.valid("customer-session", "customer", expiry) {
		t.Fatal("failed single-session revocation was not rolled back in memory")
	}
}
