package main

import (
    "crypto/hmac"
    "crypto/rand"
    "crypto/sha256"
    "crypto/subtle"
    "encoding/base64"
    "encoding/hex"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"
)

type Auth struct { Username, PasswordHash, Secret string; Enabled bool }
func NewAuth() Auth { username := os.Getenv("STEPANEL_ADMIN_USERNAME"); if username == "" { username = "admin" }; password := os.Getenv("STEPANEL_ADMIN_PASSWORD"); secret := os.Getenv("STEPANEL_SESSION_SECRET"); if secret == "" { secret = randomSecret() }; return Auth{Username: username, PasswordHash: hashPassword(password), Secret: secret, Enabled: password != ""} }
func (a Auth) Login(w http.ResponseWriter, r *http.Request) { if !a.Enabled { http.Redirect(w, r, "/", http.StatusSeeOther); return }; if r.Method == http.MethodGet { w.Header().Set("Content-Type", "text/html; charset=utf-8"); _, _ = w.Write([]byte(loginPage(""))); return }; if r.FormValue("username") != a.Username || subtle.ConstantTimeCompare([]byte(hashPassword(r.FormValue("password"))), []byte(a.PasswordHash)) != 1 { w.Header().Set("Content-Type", "text/html; charset=utf-8"); w.WriteHeader(http.StatusUnauthorized); _, _ = w.Write([]byte(loginPage("Invalid credentials"))); return }; expiry := time.Now().Add(12 * time.Hour).Unix(); payload := a.Username + "|" + strconv.FormatInt(expiry, 10); token := payload + "|" + a.sign(payload); http.SetCookie(w, &http.Cookie{Name: "stepanel_session", Value: base64.RawURLEncoding.EncodeToString([]byte(token)), Path: "/", Expires: time.Unix(expiry, 0), HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode}); csrf := randomSecret(); http.SetCookie(w, &http.Cookie{Name: "stepanel_csrf", Value: csrf, Path: "/", Expires: time.Unix(expiry, 0), Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode}); http.Redirect(w, r, "/", http.StatusSeeOther) }
func (a Auth) Logout(w http.ResponseWriter, r *http.Request) { http.SetCookie(w, &http.Cookie{Name: "stepanel_session", MaxAge: -1, Path: "/"}); http.SetCookie(w, &http.Cookie{Name: "stepanel_csrf", MaxAge: -1, Path: "/"}); http.Redirect(w, r, "/login", http.StatusSeeOther) }
func (a Auth) Require(next http.Handler) http.Handler { return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { if !a.Enabled || a.validSession(r) { next.ServeHTTP(w, r); return }; if strings.HasPrefix(r.URL.Path, "/api/") { http.Error(w, "authentication required", http.StatusUnauthorized) } else { http.Redirect(w, r, "/login", http.StatusSeeOther) } }) }
func (a Auth) CSRF(r *http.Request) bool { if !a.Enabled { return true }; cookie, err := r.Cookie("stepanel_csrf"); if err != nil { return false }; value := r.FormValue("csrf"); if value == "" { value = r.Header.Get("X-CSRF-Token") }; return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(value)) == 1 }
func (a Auth) validSession(r *http.Request) bool { cookie, err := r.Cookie("stepanel_session"); if err != nil { return false }; raw, err := base64.RawURLEncoding.DecodeString(cookie.Value); if err != nil { return false }; parts := strings.Split(string(raw), "|"); if len(parts) != 3 || parts[0] != a.Username || a.sign(parts[0]+"|"+parts[1]) != parts[2] { return false }; expiry, err := strconv.ParseInt(parts[1], 10, 64); return err == nil && time.Now().Unix() < expiry }
func (a Auth) sign(value string) string { mac := hmac.New(sha256.New, []byte(a.Secret)); _, _ = mac.Write([]byte(value)); return hex.EncodeToString(mac.Sum(nil)) }
func hashPassword(value string) string { sum := sha256.Sum256([]byte(value)); return hex.EncodeToString(sum[:]) }
func randomSecret() string { b := make([]byte, 32); if _, err := rand.Read(b); err != nil { return "development-secret-change-me" }; return base64.RawURLEncoding.EncodeToString(b) }
func loginPage(message string) string { return `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sign in · StePanel</title><style>body{font:15px system-ui;background:#f5f7f2;color:#192522;display:grid;place-items:center;min-height:100vh}main{background:#fff;border:1px solid #dfe6df;padding:36px;width:min(360px,calc(100% - 40px))}input,button{display:block;width:100%;height:44px;margin:12px 0;padding:0 12px;box-sizing:border-box}button{background:#192522;color:#fff;border:0}</style></head><body><main><h1>StePanel</h1><p>` + message + `</p><form method="post"><input name="username" autocomplete="username" placeholder="Username" required><input name="password" type="password" autocomplete="current-password" placeholder="Password" required><button>Sign in</button></form></main></body></html>` }
