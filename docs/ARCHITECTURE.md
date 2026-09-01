# Architecture

StePanel is deliberately small at this stage. The Go process owns the control-plane HTTP API and server-rendered dashboard. The operating system owns the LAMP services and systemd lifecycle.

The container image packages only the StePanel control plane. Apache, MySQL/MariaDB, PHP, and site files remain external concerns in container deployments.

```text
Browser
   │ HTTPS via Apache or another reverse proxy
   ▼
StePanel HTTP server
   ├── dashboard and static assets
   ├── health endpoint
   └── cpmove inspection/import API
          ├── staging area: /var/lib/ste-panel/imports
          ├── site roots: /var/www/sites/<account>/public
          └── MySQL client → MySQL/MariaDB socket
```

## Import safety model

1. The upload is size-limited.
2. gzip and tar streams are parsed before any restore.
3. Absolute and parent-traversing archive paths are rejected.
4. The archive is copied to a private, timestamped staging directory.
5. Website files are copied to a target-specific root.
6. SQL restore is opt-in, uses account-prefixed database names, refuses an
   existing destination database, and removes newly created databases when a
   restore fails.

When configured, StePanel provides administrator authentication with signed,
expiring sessions, CSRF tokens for mutating forms, login rate limiting,
security response headers, and JSONL audit events. TLS and a reverse proxy are
still required before internet-facing production use. Apache snippets and
systemd application units cross narrowly validated, root-owned helper
boundaries; the service account cannot edit active configuration directly.
Local database administration crosses a root-owned helper boundary. Uploaded
SQL is executed through an ephemeral account with privileges only on the newly
created target schema. Remote database deployments use the explicitly supplied
credential and remain an operator-managed trust boundary.

## Durable restore state

Restore and certificate jobs are recorded atomically before background work
starts. A restart preserves completed results and marks work interrupted by an
unclean shutdown as failed. Site overwrites use a transaction journal on the
site filesystem: the previous document root is moved into the recovery
transaction before deployment. Newly created database names and users are
journaled in the same transaction before provisioning. Startup removes managed
databases first and then rolls back every uncommitted site transaction.
Recovery artifacts are retained for the same period as restore staging data.
