# Feature catalog

StePanel is a small control plane for operators moving workloads from cPanel
to a LAMP server. This page describes shipped behavior separately from the
longer-term hosting-panel roadmap.

## Available now

- Go HTTP control plane with signed administrator sessions.
- CSRF protection, login rate limiting, security headers, and JSONL audit logs.
- cPanel `cpmove` inspection with archive traversal and size checks.
- Asynchronous website and optional SQL restore jobs.
- MySQL/MariaDB selection during installation.
- ModSecurity and optional OWASP CRS installation in DetectionOnly mode.
- Live service inventory for Apache, PHP-FPM, MySQL/MariaDB, Fail2Ban, and ModSecurity.
- Authenticated security posture endpoint at `/api/security/audit`.
- Prometheus-compatible metrics, Docker packaging, Helm, Kubernetes, and Terraform examples.

## Planned hosting features

- Site and domain lifecycle management.
- PHP-FPM pools and per-site PHP version selection.
- Verified scheduled backups and snapshot-backed rollback.
- Role-based access, MFA/OIDC, and a restricted privileged restore helper.

Planned operations will be introduced behind explicit permissions and dry-run
modes. The project will not silently mutate live web-server configuration.
