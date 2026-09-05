# Production gap analysis

This document records what is required to operate StePanel safely and what is
required to turn it into a shared-hosting customer panel. It prevents a secure
single-host operator tool from being mistaken for a complete multi-tenant
platform.

## Implemented production controls

- Authentication has secure session cookies, CSRF protection, login
  throttling, optional TOTP, security headers, and tamper-evident audit logs.
- Restore archives are inspected before extraction and restore work is
  asynchronous.
- Backups can be independently verified. Deployments can require an offsite
  backup target with `STEPANEL_REQUIRE_OFFSITE_BACKUP=1`.
- Container and deployment examples use immutable image references. CI pins
  third-party actions, performs a production startup smoke test, and checks
  Kubernetes/Helm/Terraform references.
- Kubernetes examples include a disruption budget and ingress-only network
  policy. TLS termination is explicit; the application does not silently
  trust a proxy.
- Prometheus metrics and alert rules are included for availability, restore
  failures, and stuck jobs.
- Operators have site-centric inventory, constrained pre-built HTTPS Git
  deployment with atomic file rollback, and credential-safe database detail.

## Required before exposing the panel to customers

### Identity and tenancy

Add a durable tenant/account model, scoped RBAC, API-token management,
OIDC/WebAuthn, session revocation, approval workflows for destructive actions,
and tenant-aware audit/event records. Every object and background job must be
authorized against the tenant at the data-access boundary, not only in HTTP
handlers.

### Durable control plane

Move mutable state and job state from host-local JSON/in-process memory to a
versioned relational database and durable queue. Jobs need idempotency keys,
leases, retries with backoff, cancellation, progress, dead-letter handling,
and an independently monitored worker/agent fleet. Agents must use scoped
credentials and mutual authentication.

### Hosting lifecycle

Implement complete, transactional lifecycle operations for domains/DNS/ACME
certificates, sites, PHP runtimes, databases/users, mailboxes/forwarding,
FTP/SFTP, cron, SSH keys, quotas, service plans, and billing entitlements.
Deletion must be a documented cascade with retention and recovery semantics;
removing only a vhost is not account deletion.

### Customer experience

Add a customer portal, file manager, Git-provider webhooks and deploy keys,
sandboxed build pipelines, WordPress updates and staging, backup
browsing/restore, notifications, API/webhooks, and clear operation progress.
All customer-visible operations should be
idempotent and explain what changed.

## Release gates

Before a shared-hosting launch, require an external security review, tenant
isolation tests, restore drills, upgrade/rollback drills, load and failure
testing, documented RPO/RTO, on-call ownership, data-retention policy, and a
support/compatibility policy. A deployment is not production-ready merely
because its container starts or its health endpoint is green.
