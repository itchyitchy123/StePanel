# Developer workflows

StePanel keeps application code execution outside the control-plane process.
Site configuration and lifecycle actions are authenticated, audited control
plane operations; package installs and source builds run as the isolated site
identity or inside the dedicated Podman runner.

## Environment and runtime

Configure encrypted variables with `PUT /api/sites/environment/{site}`. Secret
values are never returned after write. Updates render a root-owned systemd
environment file and restart managed Node, Python, and worker services. Use
`GET`/`PUT /api/sites/php/{site}` to inspect installed FPM versions and apply a
validated site profile for memory, execution, uploads, input limits, OPcache,
errors, and version-specific socket selection.

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
timeout, and inspect service output through the `cron` log source. This is
deliberately not a writable server crontab.

## Staging (Preview/Beta)

`POST /api/staging` creates a recovery-journaled staging site and route from
safe regular-file copies. It may copy non-secret environment variables, but
never production secrets. Database cloning, Basic Auth, no-index headers, and
outbound-email blocking remain planned integrations so staging cannot silently
expose data.

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
