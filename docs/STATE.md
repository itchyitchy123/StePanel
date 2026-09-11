# StePanel state and disaster-recovery contract

This document is the canonical inventory for StePanel state. The production
control-plane database is authoritative; legacy JSON files are migration inputs
only and must not be treated as an independent live source after migration.

| State | Canonical storage | Backup | Secret-bearing | Recovery action |
| --- | --- | --- | --- | --- |
| Jobs, leases, retries, cancellation | `STEPANEL_CONTROL_PLANE_DB` (`jobs`) | Required | May contain encrypted payloads | Restore DB, then reconcile leases |
| Customer accounts and site ownership | Control-plane DB (`accounts`, `tenant_sites`) | Required | Password hashes, encrypted TOTP | Restore DB and `STEPANEL_ACCOUNT_KEY` |
| Sessions and API tokens | Control-plane DB (`sessions`, `api_tokens`) | Required | Session/token hashes | Restore DB; rotate credentials if exposed |
| TOTP replay counters | Control-plane DB (`totp_replay`) | Required | No | Restore DB; stale codes remain rejected |
| Transactional panel state | Control-plane DB (`state_blobs`) | Required | Depends on feature | Restore DB and encryption keys |
| Audit chain | `STEPANEL_AUDIT_LOG` and `.state` | Required | HMAC key required | Restore together with audit key and verify |
| Runtime configuration | `/etc/ste-panel.env` or deployment secret store | Required | Yes | Restore with matching keys |
| Site recovery journals | `STEPANEL_RECOVERY_ROOT` | Required during active changes | No | Start panel and reconcile journals |
| Site data and local backups | `STEPANEL_WEB_ROOT`, `STEPANEL_BACKUP_ROOT` | Required | Customer data | Restore files and database data separately |
| Git trust and deploy keys | `/etc/stepanel/git-known-hosts`, `/etc/stepanel/git-keys` | Review/regenerate | Yes | Review or regenerate per site |
| rclone/provider configuration | `RCLONE_CONFIG` and provider secret store | External requirement | Yes | Re-provision and validate destination |
| Legacy JSON stores | Paths beside `STEPANEL_JOB_STATE` | Migration-only | Some are sensitive | Preserve during upgrade until migration verified |

The current legacy paths (`jobs.json`, `sessions.json`, `accounts.json`, and
feature-specific JSON files) remain configurable for one-time import and
backward-compatible tooling. New production mutations use the SQLite database.

## Required backup procedure

1. Stop or drain panel and worker mutations for a maintenance-window backup.
2. Run `stepanel backup-control-plane DEST` and then `restore-control-plane DEST --dry-run`.
3. Preserve the database's WAL/SHM files when copying a live SQLite database,
   or use the verified control-plane backup command instead.
4. Preserve the audit log, audit state, audit key, runtime configuration,
   encryption keys, site recovery journals, site data, and verified backups.
5. Validate an offsite copy and perform a restore on a disposable host.

`dr-check` inventories these obligations without copying secret values. It is
an inventory gate, not proof that a blank host has been recovered. Production
acceptance additionally requires destructive power-loss, disk-exhaustion,
database-failure, offsite-transfer, and blank-host restore drills.
The required fault matrix and assertions are defined in
[`DESTRUCTIVE_RECOVERY_LAB.md`](DESTRUCTIVE_RECOVERY_LAB.md).

## Restore contract

Control-plane restores are forward-compatible only. Restore the database with
the binary and encryption keys that created it, run the candidate's startup
migrations, and verify readiness, audit continuity, job state, and managed
site reconciliation before deleting the pre-restore copy. Downgrades require
restoring the previous binary and its compatible database backup; automatic
schema downgrade is not supported.
