# Developer workflows

StePanel keeps application code execution outside the control-plane process.
Site configuration and lifecycle actions are authenticated, audited control
plane operations; package installs and source builds run as the isolated site
identity or inside the dedicated Podman runner.

## Environment and runtime

Configure encrypted variables with `PUT /api/sites/environment/{site}`. Desired
values are persisted before host application; if helper application is
interrupted, startup reconciliation retries the persisted values. Secret
values are never returned after write. Updates render a root-owned systemd
environment file and restart managed Node, Python, and worker services. Use
`GET`/`PUT /api/sites/php/{site}` to inspect installed FPM versions and apply a
validated site profile for memory, execution, uploads, input limits, OPcache,
errors, and version-specific socket selection.

## Resource profiles (Preview/Beta)

Administrators can `GET`/`PUT /api/sites/resources/{site}` to apply an
enforceable profile for managed Node/Python application and worker processes:
CPU quota/weight, `MemoryHigh`/`MemoryMax`, I/O weight, task/PID maximum, and
isolated PHP-FPM `pm.max_children`. The helper renders a root-owned per-site systemd slice and
attaches managed application/worker units to it. The desired profile is saved
as `pending` before host application and becomes `applied` only after both the
systemd slice and FPM pool validate successfully.
Scheduled-task units are also assigned to this slice, so CPU, memory, I/O, and
task ceilings apply consistently to cron-like work when a site profile exists.

`GET /api/sites/resources/{site}` also reports observed systemd-slice state.
Administrators can call `POST /api/reconcile/resources` to re-apply pending or
inactive desired profiles after a helper failure or host restart; each repair
is audited.

`GET /api/sites/usage/{site}` reports bounded regular-file bytes, files, and
directories without following symlinks. It intentionally reports measured
usage—not an enforced disk/inode quota—until a filesystem quota provider is
configured.

Administrators can use `GET /api/security/center` for a consolidated,
read-only host posture view: existing authentication/privileged-helper checks,
service states, free disk/inodes, and backup schedule failures. It deliberately
does not mutate firewall, package, SSH, or Fail2Ban state.

This does **not** yet impose disk/inode/project quotas, network/bandwidth or
block-I/O limits, database limits, or Redis ACL/memory enforcement. Those need
host/provider-specific controls before they can be presented as tenant limits.
FPM Lens remains review-first: use its evidence-backed target/max recommendation
as an administrator input; StePanel does not auto-apply a Lens plan.

## Composer and Node

`GET /api/composer/{site}` reports Composer, `composer.json`, `composer.lock`,
and the last successful operation. `POST /api/composer/{site}/install` runs a
fixed, non-interactive install as the site user. It disables scripts and
plugins; use the build runner for repository-controlled build behavior.

`POST /api/node/tooling` supports fixed npm, Yarn, and pnpm install/build
actions as the site user. Node application start/restart remains managed by the
application lifecycle helper.

## Sandboxed builds (Shipped)

`POST /api/runner/build` accepts a site, OCI image, and up to 16 bounded build
commands. The rootless Podman runner mounts source read-only and artifact output
writable, drops capabilities, uses a read-only container filesystem and a
separate network namespace. It does not activate a release: review the artifact
and use the existing atomic deployment workflow for activation/rollback.
`GET /api/deployments?site={site}` provides durable, site-scoped build and
activation records (commit, artifact path, preserved release, stage, outcome,
and timestamp). This is the release-object foundation; fully automated
build-to-activation and database migration orchestration remains Preview work.

## Release pipeline (Preview/Beta)

`POST /api/deployments/run` connects the safe pieces in one authenticated
operation: it performs an allowlisted Git checkout into a non-live release
directory, optionally creates a verified files-and-managed-database backup,
runs the supplied bounded commands in the rootless runner, verifies the build
artifact, and atomically activates it while preserving the previous release.

Build commands must write the complete deployable release tree to `/artifact`.
The checked-out source is mounted read-only, so a build cannot modify it in
place. A failed checkout, backup, build, or artifact validation leaves the live
release untouched. Application rollback restores the previous files only; it
does **not** reverse database migrations. Declarative database migration and
post-activation health-check stages remain planned before this endpoint is
appropriate for unattended production deployment.

Git activation and rollback retain the newest previous release and prune older
validated StePanel release directories according to
`STEPANEL_GIT_RELEASE_RETENTION` (default `3`, range `1..100`). Retention
never considers the active `public` tree for deletion.

## Git deploy keys (Shipped)

`POST /api/sites/git-key/{site}` creates a read-only per-site ED25519 deploy
key and returns its public half. Add that public key to GitHub, GitLab, or
Bitbucket, then deploy an allowlisted `git@host:owner/repository.git` source.
The private half never enters Go state, the API response, or a build container;
root-owned `stepanel-gitctl` uses it only for a non-interactive clone.

## Scheduled tasks (Shipped)

`GET`/`PUT`/`DELETE /api/tasks/{site}/{name}` manages a bounded site-identity
systemd service and timer. Use a systemd `OnCalendar` expression (for example,
`*-*-* *:*:00` for each minute), select PHP/Node/Python/shell, provide a
timeout, and inspect service output through the `cron` log source. Task scripts
are root-owned under `/var/lib/stepanel/tasks/{site}` so the site identity can
execute but cannot silently rewrite the audited command. This is deliberately
not a writable server crontab.
If a helper or persistence step is interrupted, the task remains `pending` and
is retried during startup or through administrator-only
`POST /api/reconcile/tasks`.

## Staging (Preview/Beta)

`POST /api/staging` creates a recovery-journaled staging site and route from
safe regular-file copies. It may copy non-secret environment variables, but
never production secrets. Staging applies `X-Robots-Tag: noindex, nofollow` by
default through the managed webserver route; send `"no_index":false` only for
an explicitly reviewed exception. Set `basic_auth:true`, `auth_user`, and an
initial `auth_password` to protect the route with Basic Auth; only a bcrypt
hash is persisted by the Apache/Caddy helper and the password is discarded
after the request. Database cloning and outbound-email blocking remain
planned integrations.

`POST /api/backups/restore-to-staging` restores a fully verified backup's
regular site files into a new isolated site and validated route. It deliberately
refuses existing destinations and never restores database dumps; use it to
inspect a recovery point before deciding on a production change.

## Access, logs, and workers

Site access APIs manage validated SSH public-key fingerprints plus SFTP/shell
policy. Keys are never private-key material. Site logs are available through
`GET /api/sites/logs/{site}` with source allowlisting, filtering, bounded line
counts, and download support. Worker definitions create fixed-command systemd
units for Laravel queues/Horizon, Node, Celery, or RQ with restart and resource
limits.

## WordPress and Redis (Preview/Beta)

WordPress operations use a closed WP-CLI action set for updates, maintenance
mode, and due cron execution. Redis/Valkey allocations persist logical database,
namespace, memory-policy, and eviction metadata; host ACL and quota enforcement
requires the privileged provider integration before untrusted tenancy.
