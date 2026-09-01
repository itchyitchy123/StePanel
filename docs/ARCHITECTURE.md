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
Database administration uses a dedicated credential stored in the
root-readable service environment.
