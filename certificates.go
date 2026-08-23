package main

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"os/exec"
	"strings"
)

func (a *App) issueCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !a.Auth.CSRF(r) {
		http.Error(w, "invalid request", http.StatusForbidden)
		return
	}
	var input struct{ Domain, Email string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	input.Domain = strings.ToLower(strings.TrimSpace(input.Domain))
	input.Email = strings.TrimSpace(input.Email)
	if !domainPattern.MatchString(input.Domain) {
		http.Error(w, "invalid domain", 422)
		return
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email {
		http.Error(w, "invalid email address", 422)
		return
	}
	if a.Config.Certbot == "" {
		http.Error(w, "certificate helper is not installed", 503)
		return
	}
	if err := exec.Command(a.Config.Certbot, input.Domain, input.Email).Run(); err != nil {
		http.Error(w, "certificate issuance failed", 502)
		return
	}
	_ = Audit(a.Config.AuditLog, "certificate.issued", input.Domain, "Let's Encrypt certificate requested")
	writeJSON(w, http.StatusAccepted, map[string]string{"domain": input.Domain, "status": "issued"})
}
