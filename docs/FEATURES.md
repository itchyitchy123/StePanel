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
- Scheduled local backup jobs with retention controls and optional enforced
  offsite-target policy.
- Site restore/delete operations, including deterministic site IDs and
  per-site Node.js runtime selection where supported by the target host.
- Production deployment examples with immutable image/action references,
  readiness smoke tests, Kubernetes disruption/network controls, and release
  provenance/SBOM generation.

## Partial or operator-only features

- Site deletion currently removes the managed vhost/proxy state; it is not yet
  a complete customer/account teardown across mail, DNS, databases, quotas,
  and external providers.
- Backup verification is available, but snapshot-backed rollback and a full
  customer self-service restore workflow are not complete.

## Not yet production-complete for shared hosting

StePanel is currently a single-administrator, single-host control plane. It is
not yet a cPanel/Plesk-equivalent multi-tenant hosting product. The following
must be implemented before offering untrusted customer access:

- Tenant/account isolation, scoped roles, API tokens, OIDC/WebAuthn, and
  approval/audit workflows.
- Durable relational state and a distributed job/agent model for multiple
  servers, retries, cancellation, idempotency, and event delivery.
- Complete domain/DNS/SSL, database/user, mail, FTP/SFTP, cron, SSH, quota,
  resource-plan, and billing lifecycle management.
- Customer-facing file manager, deployment/Git integration, WordPress
  lifecycle tooling, notifications, and self-service backup/restore.

These are product and architecture work items, not safe one-file patches. The
sequencing, acceptance gates, and operational prerequisites are tracked in
[`PRODUCTION_GAP_ANALYSIS.md`](PRODUCTION_GAP_ANALYSIS.md).

Planned operations will be introduced behind explicit permissions and dry-run
modes. The project will not silently mutate live web-server configuration.
