package main

import (
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
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
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	TOTPSecret   string    `json:"totp_secret"`
	Plan         string    `json:"plan"`
	Sites        []string  `json:"sites"`
	Suspended    bool      `json:"suspended"`
	CreatedAt    time.Time `json:"created_at"`
}

// AccountStore holds customer identities and site assignments. Administrator
// credentials remain environment-managed and are deliberately never copied to
// this file.
type AccountStore struct {
	mu       sync.RWMutex
	path     string
	accounts map[string]HostingAccount
}

func OpenAccountStore(path string) (*AccountStore, error) {
	store := &AccountStore{path: path, accounts: make(map[string]HostingAccount)}
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
// assignments; termination is intentionally a separate destructive operation.
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
	return account, nil
}

func (s *AccountStore) Delete(username string) error {
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
	return account, nil
}

func (s *AccountStore) persistLocked() error {
	accounts := make([]HostingAccount, 0, len(s.accounts))
	for _, account := range s.accounts {
		accounts = append(accounts, account)
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
			if err := a.Accounts.Delete(username); err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			_ = AuditAs(a.Config.AuditLog, a.Auth.UsernameForRequest(r), "hosting.account.terminated", username, "account removed")
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
