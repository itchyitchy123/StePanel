# Operations runbook

## Health check

```sh
curl -fsS http://127.0.0.1:8090/api/health | jq
```

## Logs

```sh
journalctl -u stepanel --since today
journalctl -u apache2 --since today
journalctl -u mysql --since today
```

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

## Safe maintenance

`systemctl stop stepanel` stops accepting HTTP work and waits for active restore
and certificate jobs for up to two hours before systemd forces termination.
Check `stepanel_restore_jobs_active` before package upgrades or planned reboots.
Back up `/etc/ste-panel.env`, the database server, `/var/www/sites`, and
`/var/lib/ste-panel` before upgrading.

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
