package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Auth struct {
	Username, PasswordHash, Secret, AuditLog string
	credentialKey                            string
	credentialHash                           string
	Enabled, SecureCookies                   bool
	TOTPEnabled                              bool
	totpSecret                               []byte
	totpReplay                               *totpReplayState
	loginLimiter                             *loginLimiter
	sessions                                 *sessionRegistry
}

type sessionRegistry struct {
	mu      sync.RWMutex
	path    string
	entries map[string]int64
	err     error
}

type totpReplayState struct {
	mu          sync.Mutex
	lastCounter uint64
}

func NewAuth(secureCookies bool) (Auth, error) {
	username := os.Getenv("STEPANEL_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("STEPANEL_ADMIN_PASSWORD")
	hash := os.Getenv("STEPANEL_ADMIN_PASSWORD_HASH")
	if hash == "" && password != "" {
		generated, err := hashPassword(password)
		if err != nil {
			return Auth{}, err
		}
		hash = generated
	}
	secret := os.Getenv("STEPANEL_SESSION_SECRET")
	totpValue := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(os.Getenv("STEPANEL_ADMIN_TOTP_SECRET")), " ", ""))
	var totpSecret []byte
	if totpValue != "" {
		var err error
		totpSecret, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(totpValue)
		if err != nil || len(totpSecret) < 20 {
			return Auth{}, errors.New("STEPANEL_ADMIN_TOTP_SECRET must be an unpadded base32 secret of at least 160 bits")
		}
	}
	if password == "" && hash == "" {
		return Auth{Username: username, SecureCookies: secureCookies}, nil
	}
	if secret == "" {
		return Auth{}, errors.New("STEPANEL_SESSION_SECRET must be configured when authentication is enabled")
	}
	if len(secret) < 32 {
		return Auth{}, errors.New("STEPANEL_SESSION_SECRET must be at least 32 characters")
	}
	credentialKey := hash
	if password != "" {
		// A supplied plaintext password is stable across restarts, unlike a newly
		// generated bcrypt hash (which includes a random salt).
		passwordDigest := sha256.Sum256([]byte(password))
		credentialKey = "password-digest:" + hex.EncodeToString(passwordDigest[:])
	}
	return Auth{Username: username, PasswordHash: hash, Secret: secret, credentialKey: credentialKey, credentialHash: hash, Enabled: true, SecureCookies: secureCookies, TOTPEnabled: len(totpSecret) > 0, totpSecret: totpSecret, totpReplay: &totpReplayState{}, loginLimiter: newLoginLimiter(), sessions: &sessionRegistry{entries: make(map[string]int64)}}, nil
}

func (a *Auth) ConfigureSessionStore(path string) error {
	if !a.Enabled {
		return nil
	}
	registry := &sessionRegistry{path: path, entries: make(map[string]int64)}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) > 1<<20 {
			return errors.New("session state exceeds 1 MiB")
		}
		if err := json.Unmarshal(data, &registry.entries); err != nil {
			return fmt.Errorf("decode session state: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read session state: %w", err)
	}
	now := time.Now().Unix()
	for id, expiry := range registry.entries {
		if id == "" || expiry <= now {
			delete(registry.entries, id)
		}
	}
	if err := registry.persistLocked(); err != nil {
		return err
	}
	a.sessions = registry
	return nil
}

func (s *sessionRegistry) persistLocked() error {
	if s == nil || s.path == "" {
		return nil
	}
	data, err := json.Marshal(s.entries)
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(data, '\n'), 0600)
}

func (s *sessionRegistry) add(id string, expiry int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	for candidate, candidateExpiry := range s.entries {
		if candidateExpiry <= now {
			delete(s.entries, candidate)
		}
	}
	if len(s.entries) >= 10000 {
		// Evict the earliest-expiring session before admitting a new one.
		var oldestID string
		var oldest int64
		for candidate, candidateExpiry := range s.entries {
			if oldestID == "" || candidateExpiry < oldest {
				oldestID, oldest = candidate, candidateExpiry
			}
		}
		if oldestID != "" {
			delete(s.entries, oldestID)
		}
	}
	s.entries[id] = expiry
	if err := s.persistLocked(); err != nil {
		delete(s.entries, id)
		s.err = err
		return err
	}
	s.err = nil
	return nil
}

func (s *sessionRegistry) valid(id string, expiry int64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, ok := s.entries[id]
	return ok && stored == expiry && expiry > time.Now().Unix()
}

func (s *sessionRegistry) revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
	err := s.persistLocked()
	s.err = err
	return err
}

func (a Auth) SessionPersistenceError() error {
	if a.sessions == nil {
		return nil
	}
	a.sessions.mu.Lock()
	defer a.sessions.mu.Unlock()
	return a.sessions.err
}

func hashPassword(password string) (string, error) {
	generated, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(generated), err
}

func (a Auth) Login(w http.ResponseWriter, r *http.Request) {
	if !a.Enabled {
		http.Error(w, "authentication is disabled", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginPage("", a.TOTPEnabled)))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.loginLimiter != nil && !a.loginLimiter.Allow(clientIP(r)) {
		_ = AuditAs(a.AuditLog, "unknown", "auth.login.throttled", clientIP(r), "login rate limit exceeded")
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid login request", http.StatusBadRequest)
		return
	}
	credentialsValid := r.FormValue("username") == a.Username && bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(r.FormValue("password"))) == nil
	if credentialsValid && a.TOTPEnabled {
		credentialsValid = a.consumeTOTP(r.FormValue("totp"), time.Now())
	}
	if !credentialsValid {
		actor := r.FormValue("username")
		if actor == "" {
			actor = "unknown"
		}
		_ = AuditAs(a.AuditLog, actor, "auth.login.failed", clientIP(r), "invalid credentials")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(loginPage("Invalid credentials", a.TOTPEnabled)))
		return
	}
	if a.loginLimiter != nil {
		a.loginLimiter.Reset(clientIP(r))
	}
	if err := AuditAs(a.AuditLog, a.Username, "auth.login.succeeded", clientIP(r), "administrator session issued"); err != nil {
		http.Error(w, "audit persistence is unavailable", http.StatusServiceUnavailable)
		return
	}
	expiry := time.Now().Add(12 * time.Hour).Unix()
	sessionID, err := randomSecret()
	if err != nil {
		http.Error(w, "could not create a secure session", http.StatusInternalServerError)
		return
	}
	csrf, err := randomSecret()
	if err != nil {
		http.Error(w, "could not create a secure session", http.StatusInternalServerError)
		return
	}
	payload := a.Username + "|" + strconv.FormatInt(expiry, 10) + "|" + sessionID + "|" + a.credentialFingerprint()
	token := payload + "|" + a.sign(payload)
	if a.sessions != nil {
		if err := a.sessions.add(sessionID, expiry); err != nil {
			http.Error(w, "session persistence is unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "stepanel_session", Value: base64.RawURLEncoding.EncodeToString([]byte(token)), Path: "/", Expires: time.Unix(expiry, 0), HttpOnly: true, Secure: a.SecureCookies, SameSite: http.SameSiteStrictMode})
	http.SetCookie(w, &http.Cookie{Name: "stepanel_csrf", Value: csrf, Path: "/", Expires: time.Unix(expiry, 0), Secure: a.SecureCookies, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.CSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if a.sessions != nil {
		if id := a.sessionID(r); id != "" {
			if err := a.sessions.revoke(id); err != nil {
				http.Error(w, "session persistence is unavailable", http.StatusServiceUnavailable)
				return
			}
		}
	}
	_ = AuditAs(a.AuditLog, a.Username, "auth.logout", clientIP(r), "administrator session ended")
	http.SetCookie(w, &http.Cookie{Name: "stepanel_session", MaxAge: -1, Path: "/", HttpOnly: true, Secure: a.SecureCookies, SameSite: http.SameSiteStrictMode})
	http.SetCookie(w, &http.Cookie{Name: "stepanel_csrf", MaxAge: -1, Path: "/", Secure: a.SecureCookies, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (a Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		if a.validSession(r) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				if err := AuditAs(a.AuditLog, a.Username, "http.request", r.URL.Path, clientIP(r)); err != nil {
					http.Error(w, "audit persistence is unavailable", http.StatusServiceUnavailable)
					return
				}
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "authentication required", http.StatusUnauthorized)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	})
}
func (a Auth) CSRF(r *http.Request) bool {
	if !a.Enabled {
		return true
	}
	cookie, err := r.Cookie("stepanel_csrf")
	if err != nil {
		return false
	}
	value := r.FormValue("csrf")
	if value == "" {
		value = r.Header.Get("X-CSRF-Token")
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(value)) == 1
}
func (a Auth) validSession(r *http.Request) bool {
	cookie, err := r.Cookie("stepanel_session")
	if err != nil {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) == 3 && a.sessions == nil {
		if parts[0] != a.Username || !hmac.Equal([]byte(a.sign(parts[0]+"|"+parts[1])), []byte(parts[2])) {
			return false
		}
		expiry, err := strconv.ParseInt(parts[1], 10, 64)
		return err == nil && time.Now().Unix() < expiry
	}
	if len(parts) != 5 || parts[0] != a.Username || !hmac.Equal([]byte(parts[3]), []byte(a.credentialFingerprint())) || !hmac.Equal([]byte(a.sign(strings.Join(parts[:4], "|"))), []byte(parts[4])) {
		return false
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	return err == nil && a.sessions != nil && a.sessions.valid(parts[2], expiry)
}

func (a Auth) sessionID(r *http.Request) string {
	cookie, err := r.Cookie("stepanel_session")
	if err != nil {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 {
		return ""
	}
	return parts[2]
}
func (a Auth) credentialFingerprint() string {
	key := a.credentialKey
	if key == "" || (strings.HasPrefix(key, "password-digest:") && a.credentialHash != a.PasswordHash) {
		key = a.PasswordHash
	}
	digest := sha256.Sum256([]byte(a.Username + "\x00" + key))
	return hex.EncodeToString(digest[:16])
}
func (a Auth) sign(value string) string {
	mac := hmac.New(sha256.New, []byte(a.Secret))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (a Auth) consumeTOTP(code string, now time.Time) bool {
	if len(code) != 6 || a.totpReplay == nil {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	current := uint64(now.Unix() / 30)
	for offset := int64(-1); offset <= 1; offset++ {
		candidateCounter := int64(current) + offset
		if candidateCounter < 0 {
			continue
		}
		candidate := uint64(candidateCounter)
		if subtle.ConstantTimeCompare([]byte(totpCode(a.totpSecret, candidate)), []byte(code)) != 1 {
			continue
		}
		a.totpReplay.mu.Lock()
		defer a.totpReplay.mu.Unlock()
		if candidate <= a.totpReplay.lastCounter {
			return false
		}
		a.totpReplay.lastCounter = candidate
		return true
	}
	return false
}

func totpCode(secret []byte, counter uint64) string {
	message := make([]byte, 8)
	for index := len(message) - 1; index >= 0; index-- {
		message[index] = byte(counter)
		counter >>= 8
	}
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(message)
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0f
	value := (uint32(digest[offset])&0x7f)<<24 | uint32(digest[offset+1])<<16 | uint32(digest[offset+2])<<8 | uint32(digest[offset+3])
	return fmt.Sprintf("%06d", value%1000000)
}

func loginPage(message string, totpEnabled bool) string {
	totp := ""
	if totpEnabled {
		totp = `<input name="totp" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" placeholder="6-digit authenticator code" required>`
	}
	return fmt.Sprintf(`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · StePanel</title><style>body{font:15px system-ui;background:linear-gradient(145deg,#edf6fa,#f7fbfd);color:#172b3a;display:grid;place-items:center;min-height:100vh}main{background:#fbfdff;border:1px solid #c8e3ed;box-shadow:0 16px 40px rgba(45,92,113,.10);padding:36px;width:min(360px,calc(100%% - 40px))}input,button{display:block;width:100%%;height:44px;margin:12px 0;padding:0 12px;box-sizing:border-box}button{background:#17364a;color:#fff;border:0;border-radius:5px}</style></head><body><main><h1>StePanel</h1><p>%s</p><form method="post"><input name="username" autocomplete="username" placeholder="Username" required><input name="password" type="password" autocomplete="current-password" placeholder="Password" required>%s<button>Sign in</button></form></main></body></html>`, message, totp)
}
