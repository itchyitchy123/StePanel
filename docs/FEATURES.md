# Feature catalog

StePanel is a small control plane for operators moving workloads from cPanel
to a LAMP server. This page describes shipped behavior separately from the
longer-term hosting-panel roadmap.

## Available now

- Go HTTP control plane with signed administrator sessions.
- CSRF protection, login rate limiting, optional TOTP MFA, and security headers.
- Actor-attributed, HMAC-linked JSONL audit logs with offline verification.
- cPanel `cpmove` inspection with archive traversal and size checks.
- Asynchronous website and optional SQL restore jobs.
- MySQL/MariaDB selection during installation.
- ModSecurity and optional OWASP CRS installation in DetectionOnly mode.
- Live service inventory for Apache, OpenLiteSpeed, Caddy, PHP-FPM, MySQL/MariaDB, Fail2Ban, and ModSecurity.
- Cloud inventory and audited lifecycle actions for Linode, AWS, and OpenStack, plus Linode DNS, load-balancer, and snapshot operations.
- Strict-host-key SSH server inventory with allowlisted asynchronous restart and reboot actions.
- Authenticated security posture endpoint at `/api/security/audit`.
- Prometheus-compatible metrics, Docker packaging, Helm, Kubernetes, and Terraform examples.
- Transactional webserver vhosts and reverse proxies with duplicate-domain checks; PHP vhost templates remain Apache-specific.
- Deterministic site identities and isolated PHP-FPM pools for restored sites.
- Independently verified site and registered-database backups.

## Planned hosting features

- Site deletion and per-site PHP version selection.
- Scheduled backups and snapshot-backed rollback.
- Role-based access and OIDC federation.

Planned operations will be introduced behind explicit permissions and dry-run
modes. The project will not silently mutate live web-server configuration.
