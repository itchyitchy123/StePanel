# Architecture

StePanel is deliberately small at this stage. The Go process owns the control-plane HTTP API and server-rendered dashboard. The operating system owns the LAMP services and systemd lifecycle.

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
6. SQL restore is opt-in and uses account-prefixed database names.

The current release does not provide authentication, authorization, rate limiting, or TLS. These are required before internet-facing production use.

When configured, StePanel provides signed, expiring sessions, CSRF tokens for mutating forms, security response headers, and JSONL audit events. The current restore implementation runs inside the service account and writes only to the configured site root; a dedicated privileged helper remains the next hardening step for database/socket operations.
