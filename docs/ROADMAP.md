# Product roadmap

## 0.1 — Foundation

- Authenticated operations dashboard
- PHP hosting installation with MySQL/MariaDB/PostgreSQL selection
- Safe asynchronous cpmove staging and restore
- Health, metrics, tamper-evident audit events, and release automation
- Live service inventory and authenticated security posture checks

## 0.2 — Hosting operations

- First-run setup wizard
- Site deletion and domain lifecycle completion
- Per-site PHP version selection
- General-purpose database and database-user lifecycle (shipped in 0.6 for local single-host engines)
- Customer-visible operation history and notification delivery

## 0.3 — Recovery and scale

- Snapshot-backed restore rollback for files and databases
- Scheduled backup policies with bounded local retention and RPO evidence (shipped in 0.6)
- Durable job state, retry/cancellation semantics, and worker health
- Import progress, cancellation, and retry
- Docker and distribution integration tests

## 1.0 — Production contract for operator-managed hosting

- Stable API and migration policy
- Signed multi-platform releases
- Upgrade and rollback tooling
- Full accessibility review
- Security review and documented support policy

## 2.0 — Shared-hosting platform

- Tenant/account model with quotas, service plans, scoped RBAC, API tokens,
  OIDC, and phishing-resistant MFA
- Durable relational control-plane state and multi-host agent orchestration
- Complete domain, DNS, TLS, database, mail, FTP/SFTP, cron, SSH, and billing
  lifecycle
- Database PITR/WAL or binlog orchestration, replication topology, controlled
  switchover, and externally fenced automatic failover
- Customer portal, file manager, deployment integrations, WordPress tooling,
  notifications, and self-service backup/restore
