# Shared-hosting beta

Documentation version: `main / unreleased`; this document describes the
current branch and is not a promise that the `v0.6.0` release contains every
listed capability.

StePanel can now provide constrained customer workspaces on one managed host.
This is a deliberate first boundary, not a claim of cPanel/Plesk parity.

## What is enforced

- An administrator creates a customer account with a `starter`,
  `professional`, or `agency` plan and explicit site assignments.
- Every customer account has an independent bcrypt password hash and a unique
  unpadded Base32 TOTP seed of at least 160 bits. Customer MFA is always
  required, including outside production mode.
- `starter`, `professional`, and `agency` cap assignments at 1, 5, and 25
  sites respectively. These are enforced assignment limits, not resource plans.
- Administrators can apply preview resource profiles to managed application,
  worker, and scheduled-task processes, but plans do not yet enforce disk,
  inode, bandwidth, database, or Redis entitlements.
- Customer sessions can view only assigned site workspaces and their matching
  backup and job records. They can create a verified backup or add a domain
  route only for an assigned site.
- Cloud, SSH, service, database administration, migration, application,
  certificate, security, and account-management APIs remain administrator-only.

## Administrator API

`POST /api/accounts` is administrator-only and expects:

```json
{
  "username": "acme",
  "password": "a unique password with at least 20 characters",
  "totp_secret": "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
  "plan": "starter",
  "sites": ["acme-site"]
}
```

`GET /api/accounts` returns account metadata and plan limits, never password
hashes or TOTP seeds. Accounts expose a `suspended` lifecycle flag. Administrators
can suspend or unsuspend an account with `PATCH /api/accounts/{username}` and
`{"suspended":true|false}`; suspension immediately rejects active sessions and
revokes persisted sessions for that customer. It currently suspends **panel
access**, not hosting workloads: sites, services, data, backups, and external
resources remain intact. `DELETE /api/accounts/{username}` removes only the
customer login record and its sessions; it is not a hosting-account teardown.
The operation is audited as login removal and retains assigned workloads for a
separate, reviewed lifecycle workflow.
The endpoint is intentionally documented as **login removal**; no current API
claims to terminate the associated hosting workloads.
Account data is stored in `STEPANEL_ACCOUNT_STATE`, mode `0600`, and must be
included in host backups. With `STEPANEL_ACCOUNT_KEY`, customer TOTP secrets
are AES-GCM encrypted at rest and never serialized as plaintext. Existing
legacy plaintext records can be loaded for migration and are encrypted on the
next account write. The production default is beside the session state as
`accounts.json`; configure a dedicated absolute path when needed. Administrators
can regenerate a customer's MFA secret with `POST
/api/accounts/{username}/mfa`; the new seed is returned once, existing sessions
are revoked, and the seed must be delivered through a secure channel.
Administrators can generate one-time recovery codes with `POST
/api/accounts/{username}/recovery-codes`; only bcrypt hashes are stored and the
codes are returned once.
For a full recovery, administrators use `POST /api/accounts/{username}/recover`.
It rotates the temporary password, TOTP seed, and recovery codes, marks both
`password_reset_required` and `mfa_enrollment_required`, and revokes sessions.
The customer completes the two steps through `/api/account/password` and
`/api/account/mfa`; each endpoint revokes the current session after success.

## Site environment variables

Set `STEPANEL_ENVIRONMENT_KEY` to enable encrypted site environment storage.
Use `GET`, `PUT`, and `DELETE /api/sites/environment/{site}` to inspect metadata,
replace variables, or remove them. Secret variables are encrypted at rest and
are returned only as metadata; values are never returned after they are written.
Back up the environment state file together with the encryption key.
Updates render a root-owned systemd environment file and restart managed Node,
Python, and worker services so new values take effect. PHP applications should
consume environment values through their application configuration rather than
the global FPM process environment.

## Signed Git webhooks

Set `STEPANEL_GIT_WEBHOOK_SECRET` to enable `POST /api/sites/git-webhook`.
Send `X-StePanel-Signature: sha256=<hex HMAC-SHA256>` with a JSON payload
containing `site`, `repository`, and an optional `ref`. Normal repository
allowlists and release validation still apply.

## Redis / Valkey allocations

`GET`, `PUT`, and `DELETE /api/sites/redis/{site}` manage a site’s logical
Redis/Valkey allocation. The contract includes `database` (0–15), `namespace`,
`memory_mb`, and `eviction` (`allkeys-lru` or `noeviction`). The API reports
whether `redis-server` or `valkey-server` is installed. These are allocation
records; actual ACL, namespace, and cgroup memory enforcement require a
reviewed privileged helper before use with untrusted tenants.

## Composer

`GET /api/composer/{site}` reports Composer availability, `composer.json`,
`composer.lock`, and the last successful operation. `POST
/api/composer/{site}/install` accepts `development` and `optimize_autoloader`
booleans. It executes a fixed Composer install as the site account with
non-interactive, no-script, and no-plugin flags. Application build hooks must
remain in a separately sandboxed build runner.

## Per-site PHP runtime

`GET /api/sites/php/{site}` inventories installed PHP-FPM versions and the
site’s active profile. `PUT /api/sites/php/{site}` selects a version and sets
`memory_limit`, `max_execution_time`, `upload_max_filesize`, `post_max_size`,
`max_input_vars`, `opcache`, `display_errors`, and `error_reporting`. The
root-owned helper validates the selected pool configuration before reloading
FPM; managed Caddy and Apache routes use the selected version’s socket.

## Staging sites

`POST /api/staging` creates a distinct staging site and domain from a production
site. The request supports `files` and `environment`; only non-secret environment
values are copied, and the operation uses the site recovery journal. Database
cloning deliberately fails closed until the managed database helper provides a
transactional clone operation. New staging routes support default no-index
headers and optional bcrypt-backed Basic Auth; outbound-email blocking and
database cloning remain unavailable.

## Sandboxed build runner

`POST /api/runner/build` submits a site, OCI image, and bounded command list to
the rootless Podman runner. The runner mounts source read-only and writes only
to the site artifact directory. It uses a separate network namespace, dropped
Linux capabilities, a read-only container filesystem, and a bounded temporary
area. Activation remains a separate StePanel atomic-release operation.

## Node developer tooling

`POST /api/node/tooling` accepts `{site, action, package_manager}`. Actions are
`install` or `build`; package managers are `npm`, `yarn`, and `pnpm`. The
privileged helper maps these to fixed commands, disables interactive prompts,
uses production environment settings, and runs them as the site user. It does
not accept arbitrary command text or install lifecycle scripts for pnpm.

## SSH / SFTP developer access

`GET`, `PATCH`, and `POST /api/sites/access/{site}` expose per-site access
policy and add validated SSH public keys. `DELETE
/api/sites/access/{site}/{label}` revokes a key. Private keys are never accepted
or stored, and responses expose fingerprints rather than private material.
Activation into site `authorized_keys`, SFTP-only restrictions, and shell
account enforcement must be connected to the reviewed site helper before
enabling this for untrusted tenants.

## Background workers

Workers are managed with `GET /api/workers/{site}`, `PUT
/api/workers/{site}/{name}`, and `DELETE /api/workers/{site}/{name}`. Supported
types are `laravel`, `horizon`, `node`, `celery`, and `rq`; the helper maps
these to fixed commands and creates hardened systemd units with bounded memory,
task count, restart-on-failure, and site ownership. Arbitrary worker command
text is not accepted. Worker logs are available through the site log API when
the host captures unit output into the site log directory.

## Site logs

`GET /api/sites/logs/{site}?source=...` reads up to 1,000 lines from an
allowlisted source: `access`, `error`, `php-fpm`, `php`, `application`,
`deployment`, `build`, `cron`, or `worker`. Use `lines=1..1000`, `filter=...`,
or `download=1`. Logs are read only from the site’s managed `logs` directory;
arbitrary paths and commands are not accepted. Helpers and application runners
should write their site-specific output there.

## WordPress operations

Authenticated users can query `GET /api/wordpress/status/{site}` and run the
closed set of operations through `POST /api/wordpress/{site}` with an action of
`status`, `update_core`, `update_plugins`, `update_themes`, `maintenance_on`,
`maintenance_off`, or `cron`. Actions require `wp-cli`, a valid `wp-config.php`,
site ownership, CSRF protection, and are bounded and audited. Arbitrary WP-CLI
arguments and repository build commands remain intentionally unsupported.

## Not yet available to customers

Do not market this beta as unrestricted shared hosting. It does not yet provide
mailbox/FTP lifecycle, full SFTP/SSH account enforcement, browser file
management, customer database credentials, customer self-service scheduled
tasks, DNS/registrar lifecycle, enforced disk/inode/bandwidth/I/O quotas,
billing, customer-initiated restores, database-aware staging, support
workflows, reseller roles, or a multi-host control plane. These gaps require
host-level enforcement and durable tenancy-aware state, not merely dashboard
forms. Administrator-only scheduled tasks, deploy keys, resource profiles,
Security Center, and restore-to-staging are documented in
[`FEATURES.md`](FEATURES.md) with their beta/operator boundaries.
