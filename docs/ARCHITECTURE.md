# Architecture

StePanel is deliberately small at this stage. The Go process owns the control-plane HTTP API and server-rendered dashboard. The operating system owns the webserver, PHP, database, and systemd lifecycle.

The container image packages only the StePanel control plane. Caddy/Apache/OpenLiteSpeed, MySQL/MariaDB or PostgreSQL, PHP, and site files remain external concerns in container deployments.

```text
Browser
   │ HTTPS via Caddy (default), Apache, or another reverse proxy
   ▼
StePanel HTTP server
   ├── dashboard and static assets
   ├── process liveness and persistent-storage readiness endpoints
   ├── site/database inventory and constrained Git release switching
   └── cpmove inspection/import API
          ├── staging area: /var/lib/ste-panel/imports
          ├── site roots: /var/www/sites/<account>/public
          └── Database client → MySQL/MariaDB or PostgreSQL socket
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
expiring, server-revocable sessions persisted in private control-plane state.
Password-hash rotation invalidates previously issued sessions. CSRF tokens for mutating forms, login rate limiting,
production-required replay-resistant TOTP, security response headers, and JSONL audit
events with explicit actor and target identity. Unsafe authenticated requests
must persist a preflight audit event before reaching their handler. The default
Caddy host integration provides automatic HTTPS; alternate deployments must
provide an equivalent TLS boundary. Webserver snippets and
systemd application units cross narrowly validated, root-owned helper
boundaries; the service account cannot edit active configuration directly.
Request-facing helper invocations are context-bound with bounded lifetimes and
output, preventing a wedged service command from exhausting worker capacity.
Cloud CLI children receive a filtered environment so panel session, audit, and
database secrets are not inherited.
Local database administration crosses a root-owned helper boundary. Uploaded
SQL is executed through an ephemeral account with privileges only on the newly
created target schema. Remote database deployments use the explicitly supplied
credential and remain an operator-managed trust boundary.

PHP site domains use a root-owned helper selected for Caddy or Apache. It
serializes configuration changes with Node proxy changes, rejects conflicting
managed domains, binds PHP requests to an active per-site FPM socket, validates
the complete webserver configuration, and restores the prior snippet if
validation or reload fails. Caddy migrations may translate a conservative
subset of `.htaccess`; unsupported lines are reported and apply is fail-closed
unless the operator explicitly accepts a partial conversion.

Git site deployment runs as the unprivileged control-plane account and accepts
only HTTPS repositories from an exact hostname allowlist. Checkout disables
interactive credentials, applies a time and entry limit, rejects symlinks and
special files, and removes Git metadata before activation. The final directory
switch is serialized and atomic on the site filesystem; ownership/isolation is
then restored through the site helper. Repository code is never executed as a
build step by the control plane.

## Durable restore state

Restore and certificate jobs are recorded atomically before background work
starts. A restart preserves completed results and marks work interrupted by an
unclean shutdown as failed. Site overwrites use a transaction journal on the
site filesystem: the previous document root is moved into the recovery
transaction before deployment. Newly created database names and users are
journaled in the same transaction before provisioning. Startup removes managed
databases first and then rolls back every uncommitted site transaction. Invalid
journals are quarantined for inspection and do not block unrelated recovery;
site rollback is withheld while database cleanup is incomplete. Scheduler and
cleanup goroutines are cancelled during graceful shutdown. Recovery artifacts
are retained for the same period as restore staging data.

## Verified backups

Backups are built outside the live site and recovery trees. Site files and
optional locally managed database dumps are written into a private gzip/tar
archive. The publisher records a SHA-256 digest for every regular entry and for
the complete archive, then reopens and reads the entire archive before an atomic
directory rename makes it visible. Database dumps use single-transaction mode;
the root helper only dumps databases whose ownership ledger matches the site.

This verifies artifact integrity, not application-level consistency for files
or nontransactional database tables that change during the backup. Put
`STEPANEL_BACKUP_ROOT` on a dedicated backup filesystem, replicate completed
directories off-host or to immutable storage, and run periodic restore drills.

## Admission and health

Liveness is deliberately dependency-free so a full disk does not cause a
restart loop. Readiness checks durable job persistence plus free space on import,
backup, and recovery filesystems. Restore admission separately checks both the
upload staging filesystem and the destination site filesystem. Global job slots,
per-site serialization, upload bytes, and archive-entry counts provide bounded
concurrency and work size.

Audit events are HMAC-authenticated and linked by monotonically increasing
sequence and previous-hash fields. Separately HMAC-authenticated chain state survives
log rotation and detects tail/prefix inconsistency; startup and readiness expose
audit persistence failures. This is tamper evidence, not immutable storage, so
the log, state, and key still require independent retention.

The authenticated audit-events endpoint verifies and filters the chain in one
file pass. Query cost is still proportional to retained log size, so large
installations should rotate only after verification and retain indexed exports
for historical searches.
