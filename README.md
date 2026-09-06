# StePanel

> Documentation version: `main / unreleased`. Latest stable release:
> [`v0.6.0`](https://github.com/itchyitchy123/StePanel/tree/v0.6.0). Features
> added after that tag are documented only for the current `main` branch.

![CI](https://github.com/itchyitchy123/StePanel/actions/workflows/ci.yml/badge.svg) ![Release](https://img.shields.io/github/v/release/itchyitchy123/StePanel?display_name=tag) ![License](https://img.shields.io/github/license/itchyitchy123/StePanel)

> A modern, safety-first control plane for LAMP hosting and cPanel migrations.

StePanel is an open-source server management panel written in Go. It installs Caddy by default, with Apache and OpenLiteSpeed available explicitly, plus PHP and a selectable MySQL, MariaDB, or PostgreSQL version. It provides a focused operations dashboard and imports cPanel `cpmove` backups through an asynchronous, validated workflow.

It is designed for people who want a small, understandable hosting control plane instead of a large opaque platform.

## Why StePanel?

- **Migration-focused:** move cPanel accounts into a controlled LAMP environment.
- **Safety-first:** validate archives, reject unsafe entries, stage uploads privately, and expose restore status as a job.
- **Small footprint:** a Go control plane with a limited dependency surface.
- **Operator-friendly:** clear health endpoints, tamper-evident audit events, systemd deployment, and readable documentation.
- **Flexible database layer:** choose MySQL, MariaDB, or PostgreSQL and request an exact repository/AppStream version during installation.
- **Database administration:** optionally install phpMyAdmin for MySQL/MariaDB or phpPgAdmin for PostgreSQL; the dashboard reports service health and links to the matching administrator.

## Current capabilities

| Area | Included today |
| --- | --- |
| Installation | Caddy by default, or Apache/OpenLiteSpeed, PHP, MySQL/MariaDB/PostgreSQL, optional phpMyAdmin/phpPgAdmin, Exim/Dovecot/SpamAssassin/vsftpd, systemd, Debian/Ubuntu and RHEL-family systems |
| Migration | cPanel `.tar.gz` inspection, safe staging, website, SQL, staged mailbox restore, and fail-closed `.htaccess` conversion for Caddy |
| Operations | Dashboard, health endpoint, metrics endpoint, audit log, asynchronous restore jobs |
| Shared-hosting beta | Administrator-provisioned customer accounts with independent TOTP MFA, enforced assigned-site limits, and customer-scoped site/backup/job access |
| Security | bcrypt credentials, signed sessions, CSRF protection, archive traversal checks, restricted service user |
| Delivery | Dockerfile, ARM64/AMD64 release workflow, checksums, CI, vulnerability scanning |

The authenticated `/api/doctor` endpoint runs read-only checks for the selected
webserver, database, PHP-FPM, privileged helpers, and free disk capacity.
Backup schedules are persisted beside job state and execute through the same
validated, auditable backup pipeline as manual backups.
Set `STEPANEL_OFFSITE_TARGET` to an existing rclone destination such as
`s3:my-bucket/stepanel` to upload each verified archive after local backup
creation. Credentials remain in rclone's protected host configuration and are
never stored in StePanel state.
Set `STEPANEL_CLOUD_PROVIDER` to `linode`, `aws`, or `openstack` to expose
authenticated read-only cloud inventory at `/api/cloud` for servers, DNS,
load balancers, and snapshots. Linode uses `STEPANEL_LINODE_TOKEN`; AWS and
OpenStack use their standard CLI credential environments.
The protected `/api/cloud/action` endpoint queues start, stop, reboot, and
snapshot actions using provider-native operations; poll the returned job URL
for completion.
Set `STEPANEL_SSH_SERVERS` to a comma-separated list of aliases from the
service account's SSH config to expose strict-host-key, read-only health
inventory at `/api/ssh`.
The protected `/api/ssh/action` endpoint queues only allowlisted service
restarts and host reboots and returns a persisted job URL.
For Linode installations, `/api/cloud/dns` lists records and queues validated
DNS record creation, update, and deletion operations. The provider-neutral
`/api/dns/capabilities` endpoint reports the adapter contract and whether a
provider is configured; full zone desired-state ownership and DNSSEC remain
planned rather than being implied by Linode CRUD.
Linode load-balancer backend additions and removals are available through the
asynchronous `/api/cloud/loadbalancer` endpoint.
Linode snapshots can be listed and safely deleted through
`/api/cloud/snapshots`; deletion is asynchronous and audited.

ModSecurity with optional OWASP CRS is available through the installer in
safe `DetectionOnly` mode for Apache only. Caddy (the default) and
OpenLiteSpeed report native WAF support as unavailable and require a vetted
external WAF/security proxy. See [integrations](docs/INTEGRATIONS.md).
For Apache migrations to the default Caddy stack, see the
[`.htaccess` migration guide](docs/HTACCESS_MIGRATION.md).

> **Status:** StePanel includes a constrained single-host shared-hosting beta. It is not yet a complete multi-tenant hosting platform: plans currently limit assigned sites, not host resources, and customer mail/file/database/restore lifecycle remains unavailable. Administrator resource profiles, security posture, and restore-to-staging are available with explicit beta/operator boundaries. Run it behind authenticated HTTPS and test restores against a disposable server before using production data.

## See it quickly

![StePanel current development dashboard preview](docs/assets/dashboard-preview.png)

See the [product preview](docs/SCREENSHOTS.md) for the current development
dashboard layout. The repository image is a deterministic illustration with
representative values; capture a tagged-build screenshot for host-specific
service and capability states.

Operational key backup and rotation procedures are documented in
[`docs/SECRETS.md`](docs/SECRETS.md).

### Local development

Requirements: Go 1.26+.

```sh
git clone https://github.com/itchyitchy123/StePanel.git
cd StePanel
make check
go run .
```

Open <http://localhost:8080>. Local development uses `data/imports` and `data/www`, so root access is not required.

### Container

The container packages the control plane only. It does not run Apache, PHP, or a database server inside the container. Database administrator packages are host integrations and are not installed in the container image.

```sh
docker build -t stepanel:local .
docker run --rm -p 8080:8080 \
  -e STEPANEL_ENV=development \
  -e STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \
  -e STEPANEL_SESSION_SECRET='use-at-least-32-random-characters' \
  -e STEPANEL_AUDIT_KEY='use-a-different-32-character-secret' \
  stepanel:local
```

### Server installation from a release

Use a tagged release artifact for production installation. The archive
contains the binary, installer, helpers, service files, and web assets needed
by `install.sh`:

```sh
release=v0.6.0
arch=amd64 # use arm64 on aarch64 hosts
curl -fsSLO "https://github.com/itchyitchy123/StePanel/releases/download/${release}/stepanel_${release#v}_linux_${arch}.tar.gz"
curl -fsSLO "https://github.com/itchyitchy123/StePanel/releases/download/${release}/SHA256SUMS"
grep "stepanel_${release#v}_linux_${arch}.tar.gz" SHA256SUMS | sha256sum -c -
tar -xzf "stepanel_${release#v}_linux_${arch}.tar.gz"
sudo STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \
  STEPANEL_PANEL_HOSTNAME=panel.example.com \
  STEPANEL_DB_ENGINE=mariadb \
  STEPANEL_DB_VERSION=default ./install.sh
```

The installer records the selected database engine/version, creates a restricted `stepanel` service account, writes the requested panel hostname into the selected webserver, and binds the control plane to `127.0.0.1:8090`. Caddy provisions HTTPS automatically; Apache installations must complete TLS termination before signing in.

Verify release provenance and the GitHub attestation before installing on a
production host. The checksum authenticates download integrity; the release
page's SBOM and build-provenance attestation provide the corresponding supply
chain evidence. Building from source is a developer/contributor workflow:
see [Local development](#local-development) and keep it separate from the
normal operator installation path.

## cpmove migration

1. Snapshot the destination server.
2. Open the migration center and upload a cPanel `.tar.gz` archive.
3. Choose the destination account and whether SQL should be restored.
4. Type `IMPORT` to authorize the operation.
5. Poll the returned job status until it completes or fails.
6. Review the audit log and verify the site before switching traffic.

Website files are restored to `/var/www/sites/<account>/public`. SQL dumps are restored to new account-prefixed database names; existing databases are refused. Existing destination files can be overwritten only with explicit confirmation; always snapshot first.

## WordPress `.wpress` migration

The panel also restores All-in-One WP Migration archives. Install
`wpress-extract`, WP-CLI, and a MariaDB/MySQL client on the host, then use the
WordPress migration card in the dashboard. The authenticated preflight endpoint
is `/api/wpress/preflight`. WordPress database restore is unavailable when
PostgreSQL is the selected engine because WPress archives contain MySQL-format
data.

The restore provisions a site-prefixed database and user, imports the archive,
converts the archive table prefix with WP-CLI serialized-data support, and can
replace the old site URL. Existing site files require the explicit overwrite
checkbox and should be backed up first.

## Documentation

- [Installation guide](docs/INSTALLATION.md)
- [Database operations](docs/DATABASES.md)
- [Node application deployment](docs/NODE_APPS.md)
- [Node application lifecycle](docs/APP_LIFECYCLE.md)
- [Git site deployment and rollback](docs/GIT_DEPLOYMENTS.md)
- [cpmove migration guide](docs/CPMOVE_IMPORTS.md)
- [WordPress WPress migration guide](docs/WPRESS_IMPORTS.md)
- [Architecture and safety model](docs/ARCHITECTURE.md)
- [Engineering decisions and interview walkthrough](docs/ENGINEERING_DECISIONS.md)
- [Feature catalog](docs/FEATURES.md)
- [Shared-hosting beta](docs/SHARED_HOSTING.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Malware guard](docs/MALWARE_GUARD.md)
- [HTTPS certificates](docs/CERTIFICATES.md)
- [Developer workflows](docs/DEVELOPER_WORKFLOWS.md)
- [Product previews](docs/SCREENSHOTS.md)
- [API contract](docs/openapi.yaml)
- [Release procedure](docs/RELEASING.md)
- [Operations runbook](docs/OPERATIONS.md)
- [Product roadmap](docs/ROADMAP.md)
- [Launch kit and repository metadata](docs/LAUNCH_KIT.md)
- [Portfolio direction](docs/PORTFOLIO.md)
- [Service objectives](docs/SLO.md)
- [Incident lab and recovery scenarios](docs/INCIDENT_LAB.md)
- [Demo walkthrough](docs/DEMO.md)
- [Observability bundle](observability/README.md)
- [Deployment examples](deploy/)
- [Disposable end-to-end lab](deploy/lab/README.md)
- [FPM Lens and Fail2ban integrations](docs/INTEGRATIONS.md)
- [GitHub security hardening](docs/GITHUB_HARDENING.md)
- [Contributing](CONTRIBUTING.md)
- [Support](SUPPORT.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Release artifacts](https://github.com/itchyitchy123/StePanel/releases)

## API surface

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/api/health` | Version and service health |
| `GET` | `/livez` | Process-only liveness probe |
| `GET` | `/readyz` | Job persistence and managed-filesystem readiness probe |
| `GET` | `/api/services` | Authenticated live webserver, PHP, database, Fail2Ban, and ModSecurity inventory |
| `GET` | `/api/database` | Selected database engine, service/client health, and browser-admin readiness |
| `GET` | `/api/capabilities` | Runtime feature availability, including database restore compatibility |
| `GET` / `POST` | `/api/accounts` | Administrator-only shared-hosting account inventory and provisioning |
| `PATCH` / `DELETE` | `/api/accounts/<username>` | Suspend/unsuspend or remove a customer login; workloads are retained |
| `POST` | `/api/accounts/<username>/mfa` | Regenerate encrypted customer TOTP and revoke that customer's sessions |
| `POST` | `/api/accounts/<username>/recovery-codes` | Generate one-time hashed recovery codes and revoke that customer's sessions |
| `POST` | `/api/accounts/<username>/recover` | Administrator credential recovery: temporary password, MFA, recovery codes, and session revocation |
| `POST` | `/api/account/password` | Customer completes a required password change |
| `POST` | `/api/account/mfa` | Customer completes required MFA enrollment |
| `GET` | `/api/cloud` | Authenticated Linode/AWS/OpenStack inventory for servers, DNS, load balancers, and snapshots |
| `POST` | `/api/cloud/action` | Queue a cloud server start, stop, reboot, or snapshot action |
| `GET` | `/api/cloud/dns` | List Linode DNS records |
| `GET` | `/api/dns/capabilities` | Report configured DNS adapter and supported provider contract |
| `POST` | `/api/cloud/dns` | Queue a validated Linode DNS create or update |
| `DELETE` | `/api/cloud/dns` | Queue a Linode DNS record deletion |
| `POST` | `/api/cloud/loadbalancer` | Queue a Linode load-balancer backend add/remove |
| `GET` | `/api/cloud/snapshots` | List Linode snapshots |
| `DELETE` | `/api/cloud/snapshots` | Queue Linode snapshot deletion |
| `GET` | `/api/ssh` | Read-only SSH health inventory for configured aliases |
| `POST` | `/api/ssh/action` | Queue an allowlisted SSH service restart or reboot |
| `GET` | `/api/ftp` | Authenticated vsftpd status, chroot posture, and passive-port configuration |
| `GET` | `/api/security/audit` | Authenticated configuration and security posture checks |
| `GET` | `/api/audit/events` | Verified, bounded, filtered audit/deployment history |
| `GET` | `/api/node/versions` | List installed NVM Node versions |
| `POST` | `/api/node/select` | Select an installed Node version for a managed site |
| `POST` | `/api/node/tooling` | Run an allowlisted Node package install or production build |
| `POST` | `/api/runner/build` | Run a validated build definition in the rootless Podman runner |
| `GET` | `/api/deployments` | List durable build and release activation records; optional `site` filter |
| `POST` | `/api/deployments/run` | Preview: checkout, optionally back up, sandbox-build, validate, and atomically activate a complete artifact |
| `POST` | `/api/reconcile/resources` | Re-apply pending or inactive per-site resource profiles |
| `POST` | `/api/reconcile/tasks` | Re-apply pending or remove deleted scheduled-task definitions |
| `GET` | `/api/sites/usage/<site>` | Bounded regular-file, file-count, and directory-count usage for a site |
| `GET` | `/api/security/center` | Administrator-only host posture, service, disk/inode, and backup summary |
| `GET` | `/api/composer/<site>` | Detect Composer project files and show the latest operation |
| `POST` | `/api/composer/<site>/install` | Install Composer dependencies under the site identity |
| `GET` / `PUT` | `/api/sites/php/<site>` | Inspect installed PHP runtimes or apply a validated per-site FPM profile |
| `POST` | `/api/staging` | Transactionally create a staging site from files and non-secret environment values |
| `POST` | `/api/proxy/deploy` | Generate and reload a validated reverse proxy for the selected webserver |
| `GET` | `/api/proxy` | List managed reverse proxies |
| `POST` | `/api/proxy/test` | Test a local/private application backend |
| `DELETE` | `/api/proxy/<config>` | Remove a managed reverse proxy and reload the selected webserver |
| `POST` | `/api/caddy/htaccess` | Preview or apply a fail-closed `.htaccess` conversion for a managed Caddy PHP site |
| `GET` | `/api/sites` | List managed PHP site vhosts |
| `GET` | `/api/sites/overview` | List site-centric developer workspaces and their managed resources |
| `GET` | `/api/sites/overview/<site>` | Inspect one site workspace without exposing credentials or environment values |
| `GET` | `/api/sites/environment/<site>` | List site environment metadata; secret values are masked |
| `PUT` | `/api/sites/environment/<site>` | Replace site environment variables (requires encrypted environment storage) |
| `DELETE` | `/api/sites/environment/<site>` | Remove all site environment variables |
| `POST` | `/api/sites/deploy` | Validate and route a domain to its isolated PHP-FPM pool |
| `DELETE` | `/api/sites/<config>` | Remove a managed PHP site vhost |
| `GET` | `/api/backups` | List private verified backup artifacts (`site` filter; `limit` 1–500, default 100) |
| `POST` | `/api/backups` | Queue a site backup with optional managed database dumps |
| `POST` | `/api/backups/restore-to-staging` | Restore verified site files only into a new isolated staging site |
| `POST` | `/api/certificates/issue` | Queue a validated Let’s Encrypt certificate request |
| `POST` | `/api/apps/<site>/rollback` | Roll back a managed Node app to its previous manifest |
| `POST` | `/api/sites/git-deploy` | Checkout a validated HTTPS Git ref into an atomic site release |
| `GET` / `POST` / `DELETE` | `/api/sites/git-key/<site>` | Inspect, create, or retire a root-owned per-site Git deploy key; only the public key is returned |
| `POST` | `/api/sites/git-webhook` | Deploy a signed Git payload when `STEPANEL_GIT_WEBHOOK_SECRET` is configured |
| `GET` | `/api/sites/redis/<site>` | Inspect a site Redis/Valkey allocation and service status |
| `PUT` | `/api/sites/redis/<site>` | Assign a site logical Redis database/namespace and limits |
| `DELETE` | `/api/sites/redis/<site>` | Remove a site Redis allocation |
| `GET` | `/api/sites/access/<site>` | Inspect SFTP/shell policy and SSH key fingerprints |
| `PATCH` | `/api/sites/access/<site>` | Enable or disable SFTP and shell access |
| `POST` | `/api/sites/access/<site>` | Add a validated SSH public key |
| `DELETE` | `/api/sites/access/<site>/<label>` | Revoke an SSH public key |
| `GET` | `/api/workers/<site>` | List managed site workers |
| `PUT` | `/api/workers/<site>/<name>` | Create or update a fixed-type worker service |
| `DELETE` | `/api/workers/<site>/<name>` | Stop and remove a worker service |
| `GET` | `/api/tasks/<site>` | List site-identity systemd timer definitions |
| `PUT` / `DELETE` | `/api/tasks/<site>/<name>` | Create/update or remove a bounded scheduled task |
| `GET` | `/api/sites/logs/<site>` | Read a bounded, filtered site log (`source` query required) |
| `GET` | `/api/wordpress/status/<site>` | Check WordPress and WP-CLI availability |
| `POST` | `/api/wordpress/<site>` | Run a supported audited WordPress operation |
| `POST` | `/api/sites/git-rollback` | Atomically restore the latest preserved Git site release |
| `GET` | `/api/databases/<name>` | Inspect one managed database without exposing credentials |
| `POST` | `/api/security/scan` | Scan a managed site for suspicious PHP and optionally quarantine findings |
| `POST` | `/api/cpmove/inspect` | Validate and inspect a backup |
| `POST` | `/api/cpmove/import` | Start an authorized restore job |
| `POST` | `/api/backups/verify` | Verify a published backup and its optional signed manifest without restoring |
| `GET` | `/api/jobs/<id>` | Poll restore status |
| `GET` | `/metrics` | Prometheus-compatible process metric |

## Configuration

| Variable | Purpose |
| --- | --- |
| `STEPANEL_LISTEN` | HTTP listen address |
| `STEPANEL_ENV` | Set to `production` to enforce production authentication requirements |
| `STEPANEL_ADMIN_USERNAME` | Administrator username |
| `STEPANEL_ADMIN_PASSWORD` | Administrator password |
| `STEPANEL_ADMIN_PASSWORD_HASH` | Preferred bcrypt administrator password hash; supersedes the plaintext password variable |
| `STEPANEL_SESSION_SECRET` | Persistent session-signing secret, minimum 32 characters |
| `STEPANEL_ADMIN_TOTP_SECRET` | Optional unpadded base32 secret that makes six-digit TOTP mandatory at login |
| `STEPANEL_AUDIT_KEY` | Dedicated HMAC key for tamper-evident audit records; required in production, distinct from the session secret, and generated by the installer |
| `STEPANEL_REQUIRE_OFFSITE_BACKUP` | Set to `1` when production startup must require a configured off-site backup target |
| `STEPANEL_DB_ENGINE` | `mysql`, `mariadb`, or `postgresql` during installation |
| `STEPANEL_DB_VERSION` | `default` or an exact repository version |
| `STEPANEL_INSTALL_DB_ADMIN` | Set to `1` to install phpMyAdmin for MySQL/MariaDB or phpPgAdmin for PostgreSQL |
| `STEPANEL_IMPORT_ROOT` | Private backup staging directory |
| `STEPANEL_BACKUP_ROOT` | Private published backup directory; use a dedicated backup mount |
| `STEPANEL_WEB_ROOT` | Site destination root |
| `STEPANEL_WEBSERVER` | Managed webserver: `caddy` (default), `apache`, or `openlitespeed` |
| `STEPANEL_VHOST_ROOT` | Root-owned selected-webserver snippets for managed PHP sites |
| `STEPANEL_PROXY_ROOT` | Managed reverse-proxy state directory |
| `STEPANEL_APP_ROOT` | Private managed Node application-manifest directory |
| `STEPANEL_NVM_DIR` | NVM installation root used for managed Node versions |
| `STEPANEL_MALWARE_ROOT` | Private recoverable malware-quarantine directory |
| `STEPANEL_GIT_ALLOWED_HOSTS` | Comma-separated exact hostnames allowed for public HTTPS or private SSH Git deployments; defaults to GitHub, GitLab, and Bitbucket |
| `STEPANEL_GIT_WEBHOOK_SECRET` | Shared secret for HMAC-signed Git deployment webhooks |
| `STEPANEL_GITCTL` | Absolute root-owned deploy-key/private-clone helper path |
| `STEPANEL_GIT_RELEASE_MAX_AGE_HOURS` | Maximum age for non-immediate Git rollback releases; default 168 hours |
| `STEPANEL_GIT_RELEASE_MAX_BYTES` | Maximum retained previous-release bytes per site; default 5 GiB |
| `STEPANEL_RUNNERCTL` | Absolute rootless Podman build-runner helper path |
| `STEPANEL_ENVIRONMENT_KEY` | Stable secret enabling AES-GCM encrypted site environment storage |
| `STEPANEL_ENVIRONMENT_STATE` | Private environment state path; defaults beside job state |
| `STEPANEL_REDIS_STATE` | Private Redis/Valkey allocation state path; defaults beside job state |
| `STEPANEL_AUDIT_LOG` | JSONL audit log path |
| `STEPANEL_JOB_STATE` | Durable restore and certificate job state file |
| `STEPANEL_SESSION_STATE` | Durable revocable administrator session state file |
| `STEPANEL_ACCOUNT_STATE` | Private JSON state for shared-hosting customer accounts; defaults beside session state in production |
| `STEPANEL_ACCOUNT_KEY` | Stable secret used to encrypt customer TOTP secrets in account state; required in production |
| `STEPANEL_BACKUP_SIGNING_KEY` | External HMAC-SHA256 secret for signing and verifying backup manifests; keep outside the backup root |
| `STEPANEL_RECOVERY_ROOT` | Durable site rollback transactions on the site filesystem |
| `STEPANEL_WPRESS_EXTRACT` | WPress extractor executable; production default `/usr/local/bin/wpress-extract` |
| `STEPANEL_WPCLI` | WP-CLI executable; production default `/usr/local/bin/wp` |
| `STEPANEL_TLS_CERT_FILE` | Optional production TLS certificate path; required with `STEPANEL_TLS_KEY_FILE` for direct TLS |
| `STEPANEL_TLS_KEY_FILE` | Optional production TLS private-key path; required with `STEPANEL_TLS_CERT_FILE` for direct TLS |
| `STEPANEL_TLS_TERMINATED` | Set to `1` only when a trusted HTTPS reverse proxy or cluster ingress terminates TLS before forwarding to the panel |
| `STEPANEL_DB_HOST` | Selected database host; MySQL/MariaDB use it for supported SQL imports |
| `STEPANEL_DB_USER` | Remote database user used for connectivity and supported SQL imports; local MySQL/MariaDB installs use the restricted helper |
| `STEPANEL_DB_PASSWORD` | Required password for a configured remote database user in containers or external deployments; prefer the file option on hosts |
| `STEPANEL_DB_PASSWORD_FILE` | Database credential file; packaged systemd installs use a private runtime credential automatically |
| `STEPANEL_DB_ADMIN_URL` | Local URL path for the matching database administrator; defaults to `/phpmyadmin` or `/phppgadmin` |
| `STEPANEL_DB_ADMIN_ALLOW` | Space/comma-separated IPs or CIDRs allowed to reach the Apache database-admin route; defaults to loopback only |
| `STEPANEL_MAIL_ROOT` | Private root for staged cPanel mailbox data |
| `STEPANEL_METRICS_PUBLIC` | Set to `1` only when Prometheus metrics must be unauthenticated |
| `STEPANEL_STAGE_RETENTION_HOURS` | Retention for completed restore staging directories; default `168` |
| `STEPANEL_MIN_FREE_BYTES` | Minimum free space required before accepting a restore; default `1073741824` |
| `STEPANEL_MAX_UPLOAD_BYTES` | Per-request restore upload limit, at most 20 GiB |
| `STEPANEL_MAX_ARCHIVE_ENTRIES` | Restore/backup entry-count ceiling, at most 1,000,000 |
| `STEPANEL_MAX_CONCURRENT_JOBS` | Global long-running job limit, `1`–`32`; per-site limit remains one |
| `STEPANEL_FTP_PASSIVE_MIN` / `STEPANEL_FTP_PASSIVE_MAX` | Reported FTPS passive port range; defaults to `40100`–`40200` |
| `STEPANEL_SUDO` | Optional absolute non-interactive sudo executable used for restricted helpers |
| `STEPANEL_APPCTL` | Absolute restricted Node/systemd lifecycle helper path |
| `STEPANEL_PROXYCTL` | Absolute restricted reverse-proxy helper path |
| `STEPANEL_SITECTL` | Absolute restricted site identity/PHP-FPM isolation helper path |
| `STEPANEL_VHOSTCTL` | Absolute restricted PHP-vhost helper path |
| `STEPANEL_DBCTL` | Absolute restricted local database lifecycle helper path |
| `STEPANEL_CERTBOT` | Absolute restricted certificate helper path |

Invalid numeric limits and unsafe production paths are rejected at startup
instead of silently falling back to defaults. Production state paths must be
absolute so service behavior does not depend on its working directory.

Set `STEPANEL_INSTALL_MAIL=1` during installation to install Exim, Dovecot,
and SpamAssassin.

Set `STEPANEL_INSTALL_FTP=1` to install and enable vsftpd. The panel reports
vsftpd in the service inventory. Local users are chrooted to their site root
and passive ports default to `40100-40200`; configure FTPS and create
least-privilege site users before allowing external access. Plain FTP should
only be used on a trusted management network.

Set `STEPANEL_INSTALL_NODE=1 STEPANEL_NODE_VERSIONS=20.18.0,22.14.0` to install
Node versions through NVM. The panel can select an installed version per site
and generate a validated reverse proxy for a local app backend in the selected
webserver.
Managed apps are supervised by per-site systemd units and can be started,
stopped, or restarted through the authenticated API.

Set `STEPANEL_INSTALL_SECURITY=1` to install ClamAV, inotify-based PHP
monitoring, and recoverable quarantine handling. This is a defense-in-depth
layer, not a guarantee against all malware; keep applications patched and use
least-privilege service accounts.
Mailbox contents are preserved in the private mail root and reported by the
restore job; activation still requires destination domain, mailbox, DNS, TLS,
and credential mapping.

## Roadmap

The next product milestones are first-run setup, verified backups, safer
upgrades, resource quotas, and multi-user roles. See the
[roadmap](docs/ROADMAP.md) for the full plan.

## License

StePanel is released under the [MIT License](LICENSE).
