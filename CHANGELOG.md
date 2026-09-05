# Changelog

All notable changes to StePanel are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

No unreleased changes.

## [0.5.0] - 2026-09-04

### Added

- PostgreSQL installation support with optional default package installation on
  Debian/Ubuntu and validated PostgreSQL AppStream stream selection on
  RHEL-family systems.
- PHP PostgreSQL support through the distribution `php-pgsql` package.
- Optional phpMyAdmin installation for MySQL/MariaDB and phpPgAdmin installation
  for PostgreSQL through `STEPANEL_INSTALL_DB_ADMIN=1`.
- Authenticated `/api/database` status reporting and a dashboard database
  operations panel showing engine, version, host, service state, client, and
  the matching administration UI link.

### Changed

- Database service discovery and operator diagnostics now identify the selected
  PostgreSQL service instead of assuming MySQL/MariaDB.
- Database administration URLs can be customized with
  `STEPANEL_DB_ADMIN_URL`; Apache package integrations are linked automatically,
  while Caddy and OpenLiteSpeed require an explicitly reviewed PHP route.
- Synchronized release metadata to version `0.5.0` across Go, Helm, OpenAPI,
  and release documentation.

### Fixed

- Service inventory now reports only the configured web/database stack and
  detected optional services, eliminating false production alerts for engines
  that were intentionally not installed.
- Versioned PHP-FPM and PostgreSQL systemd units are detected without exposing
  raw system-bus errors as service states.
- PostgreSQL remote credential checks now use non-interactive `psql`; cPanel
  and WordPress MySQL restore controls are clearly disabled in PostgreSQL mode.
- Database-admin URLs reject scheme-relative, traversal, query, fragment, and
  malformed paths. Installer-managed Apache routes are IP-restricted to
  loopback by default through `STEPANEL_DB_ADMIN_ALLOW` and scoped to the panel
  virtual host rather than every hosted domain.
- The database operations card now collapses correctly on narrow screens, and
  admin-console readiness requires a valid Apache configuration target.

## [0.4.0] - 2026-09-04

### Added

- Linode snapshot listing and asynchronous deletion with strict snapshot ID
  validation and audit events.

- Asynchronous Linode load-balancer backend management with strict address,
  port, weight, and resource validation.

- Asynchronous Linode DNS record management with strict domain, record, target,
  and TTL validation.

- Asynchronous, allowlisted SSH actions for configured infrastructure servers,
  including service restarts and host reboots with strict host-key checking,
  bounded timeouts, persisted jobs, and audit events.

- Strict-host-key, read-only SSH server health inventory at `/api/ssh`, with
  bounded connectivity checks for configured infrastructure aliases.

- Cloud lifecycle actions now run as persisted asynchronous jobs with bounded
  provider timeouts, failure audits, and job-status polling.

- Authenticated cloud actions for configured Linode, AWS, and OpenStack
  providers, including start, stop, reboot, and snapshot operations with
  strict resource validation and audit events.

- Read-only cloud inventory integration for Linode, AWS, and OpenStack,
  covering servers, DNS, load balancers, and snapshots through standard
  provider credentials and CLIs.

- Durable scheduled site backups with interval validation, persisted next-run
  state, the `/api/backup-schedules` API, and execution through the existing
  audited backup job pipeline.

- OpenLiteSpeed can now be selected as the installed webserver with
  `STEPANEL_WEBSERVER=openlitespeed`; Caddy is also available with the same
  installer option, service inventory recognizes `lsws` and `caddy`, and Apache
  remains the default. OpenLiteSpeed and Caddy receive dedicated proxy helpers
  with backend validation, atomic updates, configuration checks, and rollback
  on failed reload/restart.
- Persistent, server-revocable administrator sessions, password-rotation
  invalidation, request correlation IDs, a recent-jobs API/dashboard, runtime
  capability reporting, and HTTP response-class metrics.
- Authenticated administration with bcrypt password hashes, signed sessions,
  login throttling, audit logging, and protected metrics.
- Asynchronous cpmove and WordPress restore workflows with archive inspection,
  capacity checks, progress reporting, retention controls, and database import.
- Managed Node.js applications with NVM version selection, hardened systemd
  units, rollback-aware deployment, and Apache reverse-proxy lifecycle controls.
- Optional Certbot, Fail2ban, ModSecurity/OWASP CRS, FPM Lens, ClamAV malware
  quarantine, Exim/Dovecot/SpamAssassin, and FTPS integrations.
- Service-state visibility, certificate management, observability assets,
  operations runbooks, an OpenAPI specification, and end-to-end lab tooling.
- Docker, Kubernetes, Helm, and Terraform deployment assets, including pinned
  images/providers, persistent storage, health probes, and ingress support.
- Debian/Ubuntu and RHEL-family Apache configurations plus audit-log rotation.
- Transactional PHP site vhosts that route domains to active per-site PHP-FPM
  sockets and reject conflicts with existing sites or Node proxies.
- Private site backup jobs with optional ownership-scoped database dumps,
  per-entry and whole-archive SHA-256 manifests, atomic publication, and a
  repeatable offline verification command.
- Separate dependency-free liveness and persistent-storage readiness endpoints,
  plus configurable upload, archive-entry, and global job concurrency limits.
- Optional TOTP administrator MFA with accepted-code replay protection, plus
  actor/target-aware, sequence-linked HMAC audit records and offline verification.

### Changed

- The dashboard now renders only live server, security, capability, and job
  data; simulated activity and inert controls were removed. Assets are embedded
  in the binary, external fonts were removed, and keyboard, reduced-motion,
  form-label, loading, and unavailable-feature states were improved.
- API handler errors use a consistent JSON envelope, production validates every
  managed path and privileged executable path, and unauthenticated readiness
  responses omit internal filesystem details.
- The installer now requires a panel FQDN and a 12-character administrator
  password, stores only its bcrypt hash, generates a session secret, validates
  options before host mutations, and writes configuration atomically. In-place
  upgrades preserve the existing root-owned runtime values, snapshot all
  StePanel-owned files, validate Apache and the candidate health endpoint, and
  restore the previous files and service state on core installation failure.
- The installer now generates and preserves a dedicated root-only audit HMAC
  key, refuses unsafe in-place key replacement, and deployment manifests accept
  the corresponding secret plus optional TOTP enrollment material.
- Local installations use a root-owned, operation-scoped database helper; the
  long-running control plane retains no local administrative credential.
- Newly installed mail and FTP daemons remain disabled until explicitly
  activated; FTPS activation requires readable certificate and key paths.
- Long-running restore jobs are allowed to finish during graceful shutdown,
  with matching systemd stop timeouts and per-target concurrency protection.
- Restore and certificate job records are persisted atomically; uncleanly
  interrupted work is reconciled into a visible failed state at startup.
- Site overwrites now use restart-safe transaction journals on the destination
  filesystem, retaining previous and partially restored document roots for the
  configured recovery window and rolling back uncommitted work at startup.
- Host restores now provision deterministic per-site Unix identities, private
  PHP-FPM pools, isolated Node service users, and explicit control-plane ACLs;
  site workloads are not members of Apache's shared filesystem group and PHP
  temporary files remain inside the site state directory.
- Local database restores now stream through a root-owned helper using an
  importer restricted to the one new schema. Root-only pending-operation
  records and site transaction journals enable startup cleanup after interrupted
  database provisioning or a later restore crash.
- Container and orchestration deployments now use numeric non-root identity
  `10001`, read-only root filesystems, dropped capabilities, bounded resources,
  persistent writable paths, and disabled service-account token mounting.
- The project now targets Go 1.26 and uses pinned CI and vulnerability-scanner
  versions.

### Fixed

- Added a single-instance process lock, bounded admission for expensive scan
  and inspection endpoints, stronger destination symlink checks, and HSTS in
  production mode.
- Startup now acquires a process lock to prevent multiple panel instances from
  concurrently mutating the same local job, recovery, and helper state.
- Expensive malware scans and cpmove inspections have bounded concurrent
  admission, and restore copies reject symlinked destination parents.
- Startup cleanup failures are now logged instead of silently discarded, and
  WordPress URL/cache/rewrite update failures abort the restore transaction.
- Production responses now include HSTS, while restore metadata and active
  plugin/theme configuration continue to be applied only after a malware scan.
- WordPress WPress restores now read validated `package.json` metadata, restore
  the archived active plugin/theme/stylesheet selections, and decode the
  archive's base64 `.htaccess` payload into the restored document root.
- Malformed WPress multipart requests no longer panic when upload metadata is
  missing, and cpmove inspection now enforces per-entry and total decompressed
  size limits to resist archive-bomb denial of service.
- Restore file copies now open source and destination files without following
  symlinks, close descriptors on failure, and write Node version metadata
  atomically.
- Job admission is synchronized with shutdown so persistent jobs cannot race
  `Wait`/process termination, while malware scans are bounded to one active
  filesystem scan.
- Privileged site, proxy, application, certificate, restore, backup, and
  malware operations now surface audit persistence failures instead of silently
  discarding the audit event.
- Restores now refuse pre-existing database or database-user names and remove
  newly created databases, users, and staged files when later steps fail.
- Failed SQL imports drop partially imported databases instead of leaving
  inconsistent state, and orphaned upload archives are included in retention.
- Concurrent cpmove and WordPress jobs can no longer restore into the same site.
- Backups share the per-site job lock with restores, preventing an internally
  initiated backup from racing a site replacement.
- Restore admission now checks free space on both staging and destination
  filesystems instead of silently continuing when a capacity check fails.
- Authenticated mutating requests now fail closed before handler execution when
  their audit preflight event cannot be durably persisted.
- Audit persistence failures remain visible in readiness until restart, while
  signed chain-state metadata detects unauthorized tail-pointer rewrites and
  safely anchors event sequences across log rotation.
- Apache proxy snippets are rendered with valid HTTP backend URLs, tested before
  reload, and rolled back when validation or reload fails.
- PHP site vhosts and Node proxies share an Apache configuration lock and reject
  duplicate managed or pre-existing `ServerName` assignments; certificate
  issuance participates in the same lock.
- Fresh RHEL-family installation now selects the correct Apache group and
  installs an appropriate virtual-host configuration.
- Partial mail-stack installations preserve existing daemons while keeping only
  newly installed companion services disabled by default.

### Security

- Apache proxy files and systemd units are root-owned and can only be changed
  through narrowly validated sudo helpers; the unprivileged daemon no longer
  controls Apache-included configuration directly.
- The control-plane process no longer retains global database credentials for
  local installations; destructive cleanup is limited to databases registered
  by the root-owned restore helper.
- systemd services now apply strict filesystem protection, namespace and kernel
  restrictions, empty capability sets where applicable, file-descriptor/task
  limits, private temporary directories, and restrictive umasks.
- Production responses include CSP, permissions, framing, MIME-sniffing,
  referrer, and cache-control headers; login request bodies are size-limited.
- Restore destinations, proxy backends, helper arguments, credentials, archive
  contents, and minimum free-space requirements receive stricter validation.
- Secure cookies remain mandatory in production, and installation guidance now
  requires TLS termination before first sign-in.

## [0.3.0] - 2026-09-04

### Added

- OpenLiteSpeed can be selected as the installed webserver with
  `STEPANEL_WEBSERVER=openlitespeed`; service inventory recognizes `lsws` and
  the installer preserves Apache as the default.

### Fixed

- App deployment now validates and normalizes domains at the API boundary,
  preventing malformed hostnames and audit-log injection through direct API use.
- Password-based session fingerprints remain stable across restarts without
  retaining the plaintext password in the in-memory authentication state.
- Session admission is bounded with expiry-based eviction, validation uses a
  read lock, HTTP metrics record implicit 200 responses correctly, and cPanel
  restore staging IDs are collision-resistant.

### Documentation

- Added the production-readiness wiki covering architecture, operations,
  security boundaries, backup/restore procedures, deployment, observability,
  and known limitations.

## [0.1.0] - 2026-08-22

The first documented foundation release: authenticated dashboard, LAMP installer, database engine/version selection, cpmove staging and restore, health endpoints, and deployment tooling.

### Known limitations

- Authentication, authorization, and TLS termination are not included yet.
- The installer expects a pre-built `stepanel` binary.
- Database restoration requires the local `mysql` client and root/socket access.
