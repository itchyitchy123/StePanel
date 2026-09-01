# Operations runbook

## Health check

```sh
curl -i http://127.0.0.1:8090/livez
curl -fsS http://127.0.0.1:8090/readyz | jq
curl -fsS http://127.0.0.1:8090/api/health | jq
```

`/livez` reports only that the process can serve HTTP and should be used for
restart decisions. `/readyz` returns `503` when persistent job state has failed
or the import, backup, or recovery filesystem is unavailable or below
`STEPANEL_MIN_FREE_BYTES`; use it for traffic and post-upgrade checks.

## Logs

```sh
journalctl -u stepanel --since today
journalctl -u apache2 --since today
journalctl -u mysql --since today
```

Audit records include distinct `actor`, `target`, sequence, previous-hash, and
HMAC fields. Unsafe authenticated requests are recorded before their handlers;
the control plane returns `503` instead of mutating state when that preflight
record cannot be persisted. Verify the active audit segment with:

```sh
sudo /opt/stepanel/stepanel verify-audit /var/lib/ste-panel/audit.jsonl
```

The HMAC chain continues across log rotation through `audit.jsonl.state`.
Preserve rotated logs, the state file, and `/etc/stepanel-audit.key` together in
independently controlled storage. On the first upgraded write, an unsigned
legacy log is preserved as `audit.jsonl.legacy-TIMESTAMP` before the signed
chain begins. Do not rotate or replace the audit key in place: doing so makes
the existing chain unverifiable, and the installer refuses the replacement
while the key file exists.

On RHEL-family systems, the Apache and database unit names may be `httpd` and `mariadb`.

## Capacity and retention

```sh
df -h /var/lib/ste-panel /var/www/sites
du -sh /var/lib/ste-panel/imports /var/lib/ste-panel/mail /var/lib/ste-panel/quarantine
logrotate --debug /etc/logrotate.d/stepanel
```

Keep the configured free-space floor above the largest expected compressed
upload plus extraction and database working space. Audit logs rotate daily,
at 50 MiB, and retain 30 compressed rotations. Interrupted upload files and
expired restore stages are removed by the control-plane retention loop.
Restore admission checks both staging and destination filesystems. Configure
`STEPANEL_MAX_UPLOAD_BYTES`, `STEPANEL_MAX_ARCHIVE_ENTRIES`, and
`STEPANEL_MAX_CONCURRENT_JOBS` to match I/O and memory capacity; one long-running
job per site is enforced independently of the global limit.

## Safe maintenance

`systemctl stop stepanel` stops accepting HTTP work and waits for active restore
and certificate jobs for up to two hours before systemd forces termination.
Check `stepanel_restore_jobs_active` before package upgrades or planned reboots.
Back up `/etc/ste-panel.env`, the database server, `/var/www/sites`, and
`/var/lib/ste-panel` before upgrading.

For an in-place upgrade, build the candidate binary, ensure no restore or backup
job is active, then run `install.sh` without re-supplying secrets. The installer
loads the existing root-owned environment, snapshots StePanel-owned files,
waits for the old service to stop, and health-checks the candidate. If core
configuration or startup fails it restores the previous files and service
state. Package-manager and optional integration changes are not automatically
reverted; retain the host snapshot and inspect the package transaction log.

## Verified site backups

Queue a filesystem-only backup, or include databases registered to the site by
the local database helper:

```sh
curl -fsS -X POST -H 'Content-Type: application/json' \
  --data '{"site":"ACCOUNT","include_databases":true}' \
  http://127.0.0.1:8090/api/backups
/opt/stepanel/stepanel verify-backup /var/backups/stepanel/TIMESTAMP-ACCOUNT
sha256sum -c /var/backups/stepanel/TIMESTAMP-ACCOUNT/backup.tar.gz.sha256
```

API calls require the normal authenticated session and CSRF token; the command
above illustrates the request body. A backup is published only after every tar
entry and its whole-archive checksum verify. Copy the complete timestamped
directory to off-host or immutable storage and perform scheduled restore drills.
Live file writes and nontransactional database tables are not quiesced, so use
application maintenance mode or storage/database snapshots when a point-in-time
consistent backup is required. Backups are not deleted by staging retention.

Job records are persisted in `/var/lib/ste-panel/jobs.json`. Site overwrites
move the previous document root into a journaled transaction under
`/var/www/sites/.stepanel-recovery`. On startup, StePanel marks interrupted jobs
failed, removes databases recorded by uncommitted restore transactions, and
then rolls back their site files. Committed and
rolled-back transactions remain available for the configured staging-retention
period; preserve them before that deadline when investigating an incident.

Local database operations are registered under
`/var/lib/stepanel-privileged/db-managed`. Entries prefixed with `pending-` are reconciled
by the database helper before transaction-journal recovery and HTTP startup. Do not edit this root-only
registry manually; preserve the root-only `/var/lib/stepanel-privileged` tree
with system backups.

Host restores create site identities named from the site plus a stable hash.
Inspect their ownership, ACL, and PHP-FPM pools with:

```sh
getent passwd 'sp-*'
getfacl /var/www/sites/ACCOUNT
find /etc/php /etc/php-fpm.d -name 'stepanel-ACCOUNT.conf' -print 2>/dev/null
systemctl status 'stepanel-app-ACCOUNT.service'
```

Activate a restored PHP document root only after verification by calling
`POST /api/sites/deploy` with its site and domain. Managed vhosts live under
`/etc/apache2/stepanel-sites` or `/etc/httpd/conf.d/stepanel-sites`. Apache
configuration changes are serialized, syntax-tested, and rolled back on reload
failure. A domain already present in an existing vhost or Node proxy is refused.
Certificate issuance uses the same Apache lock. Route deletion refuses to
proceed while another vhost (including a TLS companion) still serves the domain;
remove that external or certificate-managed vhost first.

Upgrades from the legacy writable proxy directory disable its Apache include.
Re-deploy each managed proxy through the panel so the validated helper creates
the corresponding root-owned snippet.

## Recovery

If an import fails, preserve the timestamped staging directory, inspect the service logs, and restore from the pre-import snapshot. Do not repeatedly retry against a live destination without identifying the failure mode.
