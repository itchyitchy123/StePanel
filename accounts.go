package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HostingPlan intentionally limits the first shared-hosting release to site
// assignment. Resource accounting and billing are separate provider concerns;
// advertising limits that are not enforced by the host would be misleading.
type HostingPlan struct {
	Name      string `json:"name"`
	SiteLimit int    `json:"site_limit"`
}

var hostingPlans = map[string]HostingPlan{
	"starter":      {Name: "starter", SiteLimit: 1},
	"professional": {Name: "professional", SiteLimit: 5},
	"agency":       {Name: "agency", SiteLimit: 25},
}

type HostingAccount struct {
	Username           string    `json:"username"`
	PasswordHash       string    `json:"password_hash"`
	TOTPSecret         string    `json:"totp_secret"`
	TOTPEncrypted      bool      `json:"totp_encrypted,omitempty"`
	RecoveryCodeHashes []string  `json:"recovery_code_hashes,omitempty"`
	Plan               string    `json:"plan"`
	Sites              []string  `json:"sites"`
	Suspended          bool      `json:"suspended"`
	CreatedAt          time.Time `json:"created_at"`
}

// AccountStore holds customer identities and site assignments. Administrator
// credentials remain environment-managed and are deliberately never copied to
// this file.
type AccountStore struct {
	mu       sync.RWMutex
	path     string
	key      []byte
	accounts map[string]HostingAccount
}

func OpenAccountStore(path string, accountKey ...string) (*AccountStore, error) {
	store := &AccountStore{path: path, accounts: make(map[string]HostingAccount)}
	if len(accountKey) > 0 && strings.TrimSpace(accountKey[0]) != "" {
		h := sha256.Sum256([]byte(accountKey[0]))
		store.key = h[:]
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read account state: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, errors.New("account state exceeds 1 MiB")
	}
	var accounts []HostingAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("decode account state: %w", err)
	}
	for _, account := range accounts {
		if account.TOTPEncrypted {
			if len(store.key) == 0 {
				return nil, errors.New("encrypted account TOTP requires STEPANEL_ACCOUNT_KEY")
			}
			plain, err := decryptAccountTOTP(store.key, account.TOTPSecret)
			if err != nil {
				return nil, fmt.Errorf("decrypt account TOTP: %w", err)
			}
			account.TOTPSecret = plain
			account.TOTPEncrypted = false
		}
		if err := validateHostingAccount(account, false); err != nil {
			return nil, fmt.Errorf("invalid account state: %w", err)
		}
		if _, exists := store.accounts[account.Username]; exists {
			return nil, fmt.Errorf("duplicate account %q", account.Username)
		}
		store.accounts[account.Username] = account
	}
	return store, nil
}

func validateHostingAccount(account HostingAccount, requireCreated bool) error {
	if safeUser(account.Username) == "" || len(account.PasswordHash) == 0 || account.TOTPSecret == "" {
		return errors.New("account username, password hash, and TOTP secret are required")
	}
	if _, err := bcryptCost(account.PasswordHash); err != nil {
		return errors.New("account password hash must be valid bcrypt")
	}
	if _, err := decodeTOTPSecret(account.TOTPSecret); err != nil {
		return errors.New("account TOTP secret must be unpadded base32 with at least 160 bits")
	}
	plan, ok := hostingPlans[account.Plan]
	if !ok || len(account.Sites) > plan.SiteLimit {
		return errors.New("account plan or assigned sites are invalid")
	}
	seen := make(map[string]bool, len(account.Sites))
	for _, site := range account.Sites {
		if safeUser(site) == "" || seen[site] {
			return errors.New("account sites must be unique valid site names")
		}
		seen[site] = true
	}
	if requireCreated && account.CreatedAt.IsZero() {
		return errors.New("account creation time is required")
	}
	return nil
}

func bcryptCost(hash string) (int, error) { return bcrypt.Cost([]byte(hash)) }

func (s *AccountStore) Get(username string) (HostingAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.accounts[username]
	return account, ok
}

func (s *AccountStore) OwnsSite(username, site string) bool {
	account, ok := s.Get(username)
	if !ok {
		return false
	}
	for _, candidate := range account.Sites {
		if candidate == site {
			return true
		}
	}
	return false
}

func (s *AccountStore) GetSites(username string) []string {
	account, ok := s.Get(username)
	if !ok {
		return nil
	}
	return append([]string(nil), account.Sites...)
}

// SetSuspended changes the account lifecycle state atomically and persists it
// before returning. Suspended accounts remain recoverable and retain their
// assignments; hosting termination is intentionally a separate destructive
// workflow.
func (s *AccountStore) SetSuspended(username string, suspended bool) (HostingAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[username]
	if !ok {
		return HostingAccount{}, errors.New("account not found")
	}
	previous := account.Suspended
	account.Suspended = suspended
	s.accounts[username] = account
	if err := s.persistLocked(); err != nil {
		account.Suspended = previous
		s.accounts[username] = account
		return HostingAccount{}, err
	}
	account.PasswordHash = ""
	account.TOTPSecret = ""
	account.RecoveryCodeHashes = nil
	return account, nil
}

// RemoveLogin deletes only the customer identity. It deliberately does not
// touch assigned sites or any hosting workload.
func (s *AccountStore) RemoveLogin(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[username]
	if !ok {
		return errors.New("account not found")
	}
	delete(s.accounts, username)
	if err := s.persistLocked(); err != nil {
		s.accounts[username] = account
		return err
	}
	return nil
}

func (s *AccountStore) List() []HostingAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := make([]HostingAccount, 0, len(s.accounts))
	for _, account := range s.accounts {
		account.PasswordHash = ""
		account.TOTPSecret = ""
		account.RecoveryCodeHashes = nil
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Username < accounts[j].Username })
	return accounts
}

func decodeTOTPSecret(value string) ([]byte, error) {
	value = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(value)
	if err != nil || len(secret) < 20 {
		return nil, errors.New("invalid TOTP secret")
	}
	return secret, nil
}

func (s *AccountStore) Create(username, password, totpSecret, plan string, sites []string) (HostingAccount, error) {
	username, plan = safeUser(username), strings.ToLower(strings.TrimSpace(plan))
	if username == "" || len(password) < 20 {
		return HostingAccount{}, errors.New("username and a password of at least 20 characters are required")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return HostingAccount{}, err
	}
	account := HostingAccount{Username: username, PasswordHash: hash, TOTPSecret: totpSecret, Plan: plan, Sites: sites, CreatedAt: time.Now().UTC()}
	if err := validateHostingAccount(account, true); err != nil {
		return HostingAccount{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.accounts[username]; exists {
		return HostingAccount{}, errors.New("account already exists")
	}
	s.accounts[username] = account
	if err := s.persistLocked(); err != nil {
		delete(s.accounts, username)
		return HostingAccount{}, err
	}
	account.PasswordHash = ""
	account.TOTPSecret = ""
	account.RecoveryCodeHashes = nil
	return account, nil
}

func (s *AccountStore) ResetTOTP(username string) (HostingAccount, string, error) {
	secretBytes := make([]byte, 20)
	if _, err := io.ReadFull(rand.Reader, secretBytes); err != nil {
		return HostingAccount{}, "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[username]
	if !ok {
		return HostingAccount{}, "", errors.New("account not found")
	}
	previous := account.TOTPSecret
	account.TOTPSecret, account.TOTPEncrypted = secret, false
	s.accounts[username] = account
	if err := s.persistLocked(); err != nil {
		account.TOTPSecret, account.TOTPEncrypted = previous, false
		s.accounts[username] = account
		return HostingAccount{}, "", err
	}
	response := account
	response.PasswordHash, response.TOTPSecret, response.TOTPEncrypted, response.RecoveryCodeHashes = "", "", false, nil
	return response, secret, nil
}

func (s *AccountStore) GenerateRecoveryCodes(username string) (HostingAccount, []string, error) {
	codes := make([]string, 10)
	hashes := make([]string, len(codes))
	for i := range codes {
		buf := make([]byte, 8)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			return HostingAccount{}, nil, err
		}
		codes[i] = fmt.Sprintf("%x", buf)
		hash, err := hashPassword(codes[i])
		if err != nil {
			return HostingAccount{}, nil, err
		}
		hashes[i] = hash
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[username]
	if !ok {
		return HostingAccount{}, nil, errors.New("account not found")
	}
	previous := account.RecoveryCodeHashes
	account.RecoveryCodeHashes = hashes
	s.accounts[username] = account
	if err := s.persistLocked(); err != nil {
		account.RecoveryCodeHashes = previous
		s.accounts[username] = account
		return HostingAccount{}, nil, err
	}
	account.PasswordHash, account.TOTPSecret, account.RecoveryCodeHashes = "", "", nil
	return account, codes, nil
}

func (s *AccountStore) ConsumeRecoveryCode(username, code string) (bool, error) {
	if len(code) < 8 || len(code) > 128 {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[username]
	if !ok {
		return false, nil
	}
	for i, hash := range account.RecoveryCodeHashes {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(code)) != nil {
			continue
		}
		previous := append([]string(nil), account.RecoveryCodeHashes...)
		account.RecoveryCodeHashes = append(account.RecoveryCodeHashes[:i], account.RecoveryCodeHashes[i+1:]...)
		s.accounts[username] = account
		if err := s.persistLocked(); err != nil {
			account.RecoveryCodeHashes = previous
			s.accounts[username] = account
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func encryptAccountTOTP(key []byte, value string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sealed), nil
}

func decryptAccountTOTP(key []byte, value string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(value)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted TOTP")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}

func (s *AccountStore) persistLocked() error {
	accounts := make([]HostingAccount, 0, len(s.accounts))
	for _, account := range s.accounts {
		persisted := account
		if len(s.key) > 0 && account.TOTPSecret != "" {
			encrypted, err := encryptAccountTOTP(s.key, account.TOTPSecret)
			if err != nil {
				return err
			}
			persisted.TOTPSecret, persisted.TOTPEncrypted = encrypted, true
		}
		accounts = append(accounts, persisted)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Username < accounts[j].Username })
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(data, '\n'), 0600)
}

func (a *App) accounts(w http.ResponseWriter, r *http.Request) {
	if a.Accounts == nil {
		http.Error(w, "shared-hosting accounts are unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "/mfa") {
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/mfa")
		username := safeUser(strings.Trim(path, "/"))
		if username == "" || strings.Contains(path, "/") {
			http.Error(w, "invalid account", http.StatusBadRequest)
			return
		}
		account, secret, err := a.Accounts.ResetTOTP(username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if a.Auth.sessions != nil {
			if err := a.Auth.sessions.revokeUser(username); err != nil {
				http.Error(w, "MFA reset saved but session revocation could not be persisted", http.StatusServiceUnavailable)
				return
			}
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "hosting.account.mfa-reset", username, "TOTP regenerated and sessions revoked")
		writeJSON(w, http.StatusOK, map[string]any{"account": account, "totp_secret": secret})
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "/recovery-codes") {
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/recovery-codes")
		username := safeUser(strings.Trim(path, "/"))
		if username == "" || strings.Contains(path, "/") {
			http.Error(w, "invalid account", http.StatusBadRequest)
			return
		}
		account, codes, err := a.Accounts.GenerateRecoveryCodes(username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if a.Auth.sessions != nil {
			if err := a.Auth.sessions.revokeUser(username); err != nil {
				http.Error(w, "recovery codes saved but session revocation could not be persisted", http.StatusServiceUnavailable)
				return
			}
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "hosting.account.recovery-codes-generated", username, "one-time recovery codes generated and sessions revoked")
		writeJSON(w, http.StatusOK, map[string]any{"account": account, "recovery_codes": codes})
		return
	}
	if r.Method == http.MethodPatch || r.Method == http.MethodDelete {
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		username := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
		username = safeUser(username)
		if username == "" || strings.Contains(r.URL.Path, "//") {
			http.Error(w, "invalid account", http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodDelete {
			if err := a.Accounts.RemoveLogin(username); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if a.Auth.sessions != nil {
				_ = a.Auth.sessions.revokeUser(username)
			}
			_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "hosting.account.login-removed", username, "customer identity removed; workloads are retained")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var input struct {
			Suspended *bool `json:"suspended"`
		}
		if err := decodeJSON(w, r, 1024, &input); err != nil || input.Suspended == nil {
			http.Error(w, "suspended must be a boolean", http.StatusBadRequest)
			return
		}
		account, err := a.Accounts.SetSuspended(username, *input.Suspended)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		event := "hosting.account.unsuspended"
		if account.Suspended {
			event = "hosting.account.suspended"
			if a.Auth.sessions != nil {
				if err := a.Auth.sessions.revokeUser(username); err != nil {
					// Account state is already safely suspended and validSession also
					// checks it, so panel access is denied even if session cleanup
					// cannot be persisted. Surface the reconciliation failure.
					http.Error(w, "account suspended but session revocation could not be persisted", http.StatusServiceUnavailable)
					return
				}
			}
		}
		_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), event, username, "account lifecycle changed")
		writeJSON(w, http.StatusOK, account)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"accounts": a.Accounts.List(), "plans": hostingPlans})
	case http.MethodPost:
		if !a.Auth.CSRF(r) {
			http.Error(w, "invalid request", http.StatusForbidden)
			return
		}
		var input struct {
			Username   string   `json:"username"`
			Password   string   `json:"password"`
			TOTPSecret string   `json:"totp_secret"`
			Plan       string   `json:"plan"`
			Sites      []string `json:"sites"`
		}
		if err := decodeJSON(w, r, 8192, &input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		account, err := a.Accounts.Create(input.Username, input.Password, input.TOTPSecret, input.Plan, input.Sites)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "hosting.account.created", account.Username, account.Plan); err != nil {
			log.Printf("account created but audit persistence is unavailable: %v", err)
		}
		writeJSON(w, http.StatusCreated, account)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
