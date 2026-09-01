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

Job records are persisted in `/var/lib/ste-panel/jobs.json`. Site overwrites
move the previous document root into a journaled transaction under
`/var/www/sites/.stepanel-recovery`. On startup, StePanel marks interrupted jobs
failed and rolls back every uncommitted site transaction. Committed and
rolled-back transactions remain available for the configured staging-retention
period; preserve them before that deadline when investigating an incident.

Host restores create site identities named from the site plus a stable hash.
Inspect their ownership, ACL, and PHP-FPM pools with:

```sh
getent passwd 'sp-*'
getfacl /var/www/sites/ACCOUNT
find /etc/php /etc/php-fpm.d -name 'stepanel-ACCOUNT.conf' -print 2>/dev/null
systemctl status 'stepanel-app-ACCOUNT.service'
```

Upgrades from the legacy writable proxy directory disable its Apache include.
Re-deploy each managed proxy through the panel so the validated helper creates
the corresponding root-owned snippet.

## Recovery

If an import fails, preserve the timestamped staging directory, inspect the service logs, and restore from the pre-import snapshot. Do not repeatedly retry against a live destination without identifying the failure mode.
