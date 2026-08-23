# StePanel service objectives

These objectives are a starting point for a small production installation.
Tune them to the backup volume and hardware profile rather than copying them
blindly.

| Service indicator | Target | Measurement |
| --- | ---: | --- |
| Control-plane availability | 99.9% monthly | `stepanel_up` and HTTP probes |
| Restore acceptance latency | 99% under 2 seconds | HTTP 202 response time |
| Successful restore ratio | 99.5% monthly | completed / (completed + failed) |
| Backup import data loss | 0 bytes | restore verification and audit log |

## Alert policy

- Page when `stepanel_up == 0` for 2 minutes.
- Page when `increase(stepanel_restore_jobs_failed_total[15m]) > 0` during a
  scheduled migration window.
- Warn when `stepanel_restore_jobs_active > 0` for longer than the expected
  restore duration.

Every alert should link to an incident ticket and one of the runbooks in
[`INCIDENT_LAB.md`](INCIDENT_LAB.md). Do not put customer names, usernames, or
backup paths into metric labels.
