# Destructive recovery lab

This is a release-acceptance procedure, not a substitute for unit or recovery
drills. Run it on disposable hosts with isolated database and offsite-storage
credentials. Never run the fault injections on a customer host.

The complete state inventory and restore prerequisites are in
[`STATE.md`](STATE.md). Capture the command output, service journals, recovery
journals, `dr-check` inventory, and final assertions as release evidence.

## Required hosts

- A fresh target host for the supported operating-system/webserver/database
  combination.
- An independent database service when testing database loss.
- A disposable immutable/offsite backup target.
- A clean bare-metal restore target.

## Required scenarios

| Fault injected while work is active | Required assertion |
| --- | --- |
| `kill -9` panel or worker during extraction, site overwrite, and SQL import | A restart reconciles the journal; no half-published site or orphan managed database remains. |
| Reboot during an active restore | The restored site is either fully committed or the previous site is restored; readiness remains failed until reconciliation completes. |
| Fill the staging, destination, and control-plane filesystems | No incomplete site becomes active; job failure is durable and retry remains idempotent. |
| Stop database during import and cleanup | Temporary credentials and managed-object registry are reconciled before file rollback. |
| Fail webserver validation/reload | Previous configuration remains active and no unauthorized route is published. |
| Kill worker with an active lease | Lease expiry is requeued or dead-lettered according to retry policy; no job silently disappears or commits twice. |
| Interrupt rclone/offsite transfer | The object is not advertised as a verified remote backup and a later retry creates a complete verified object. |
| Corrupt a copy of `STEPANEL_CONTROL_PLANE_DB` | Readiness fails; the documented control-plane restore procedure returns the host to a verified state. |
| Corrupt a recovery journal | The malformed entry is quarantined, independent valid entries reconcile, and readiness remains failed for operator action. |

For every scenario, explicitly verify:

1. No unintended public route or half-published document root exists.
2. No orphan database user, schema, or privileged registry entry exists.
3. Job state, audit continuity, and retry count match the expected terminal state.
4. `/readyz` remains non-ready for unresolved recovery or dead-letter state.
5. Repeating recovery and retry actions does not duplicate the committed result.

## Bare-host restore

At least once per release candidate, install the tagged artifact on a blank
supported host, restore the control plane, audit chain, keys, site data, and
database data from the inventory, then verify authentication, a managed site,
backup verification, and a worker job. Record the OS, webserver, database,
artifact version, elapsed recovery time, and any manual steps.

Release acceptance requires zero unrecoverable states in the exercised matrix.
An untested scenario is a release exception that must be recorded in the
release notes and support contract.

The repository additionally executes a child-process termination test for a
site transaction and a deliberately broken installer-candidate rollback test.
Those gates exercise the same persistence and installer transaction boundaries,
but do not replace the host, provider, or storage fault scenarios above.
