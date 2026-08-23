package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/mail"
	"os/exec"
	"strings"
	"time"
)

type CertificateResult struct {
	Domain string `json:"domain"`
	Status string `json:"status"`
}

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
	jobID := time.Now().UTC().Format("20060102-150405.000000000") + "-" + strings.ReplaceAll(input.Domain, ".", "-")
	if !a.Jobs.SubmitCertificate(jobID, input.Domain, func() (CertificateResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := exec.CommandContext(ctx, a.Config.Certbot, input.Domain, input.Email).Run(); err != nil {
			return CertificateResult{}, err
		}
		_ = Audit(a.Config.AuditLog, "certificate.issued", input.Domain, "Let's Encrypt certificate requested")
		return CertificateResult{Domain: input.Domain, Status: "issued"}, nil
	}) {
		http.Error(w, "too many long-running jobs", http.StatusTooManyRequests)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "queued"})
}
