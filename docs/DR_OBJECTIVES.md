# Disaster-recovery objectives

These are the default objectives for a controlled, single-host StePanel
deployment. They are not automatic product guarantees: an operator must deploy
the required backup schedule, offsite destination, immutable retention, and
perform the drills below before claiming compliance.

| Recovery scope | RPO | RTO | Preconditions and measurement |
| --- | ---: | ---: | --- |
| Control plane, audit continuity, and runtime configuration | 24 hours | 60 minutes | A verified encrypted offsite control-plane backup, matching keys, and a blank-host restore drill. Measure from declared incident to `/readyz` plus audit verification. |
| One managed site and its registered logical database dumps | 24 hours | 4 hours | A successful scheduled verified offsite backup, sufficient staging/destination capacity, and a restore drill at representative site size. Measure to a verified, non-public staging restore or explicitly approved activation. |
| Entire host fleet member | 24 hours | 8 hours | A replacement host, release artifact, all state in [`STATE.md`](STATE.md), provider access, and a bare-host restore drill. Measure to panel readiness and one representative site verification. |

The 24-hour RPO applies only when backup schedules succeed at least daily and
the latest verified artifact is copied to the independent offsite target. The
RPO is **undefined** for a site without a successful backup in that interval.
Logical database dumps and live-file archives are classified as
`crash-consistent / logical backup`; they do not promise a transactionally
quiesced application point in time. Workloads needing a smaller RPO or
application-consistent recovery require operator-managed database PITR,
filesystem snapshots, and application quiesce hooks.

## Release evidence

For every release candidate, record the actual elapsed RTO and achieved RPO
for the bare-host and destructive scenarios in
[`DESTRUCTIVE_RECOVERY_LAB.md`](DESTRUCTIVE_RECOVERY_LAB.md). A result exceeding
an objective is a release exception, not a silently accepted pass. Monitor
`stepanel_backup_oldest_age_seconds`,
`stepanel_backup_schedules_without_success`, and
`stepanel_backup_consecutive_failures`; page before a scheduled backup age can
violate the declared RPO.
