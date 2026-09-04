package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
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
	if err := decodeJSON(w, r, 4096, &input); err != nil {
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
	jobID, err := newJobID("certificate")
	if err != nil {
		http.Error(w, "could not create certificate job", http.StatusInternalServerError)
		return
	}
	if err := a.Jobs.SubmitCertificate(jobID, input.Domain, func() (CertificateResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := helperCommandContext(ctx, a.Config, a.Config.Certbot, input.Domain, input.Email).Run(); err != nil {
			return CertificateResult{}, err
		}
		if err := AuditAs(a.Config.AuditLog, a.Auth.Username, "certificate.issued", input.Domain, "Let's Encrypt certificate requested"); err != nil {
			return CertificateResult{}, fmt.Errorf("certificate issued but audit persistence is unavailable: %w", err)
		}
		return CertificateResult{Domain: input.Domain, Status: "issued"}, nil
	}); err != nil {
		if errors.Is(err, ErrJobBusy) {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
		} else {
			http.Error(w, "could not persist certificate job", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "queued"})
}
