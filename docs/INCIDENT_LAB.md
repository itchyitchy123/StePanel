# StePanel incident lab

This is the operational proof plan for the future **SteMigrate** product.
Each scenario can be reproduced on an isolated disposable VM with a synthetic
backup. Never test restore failure modes against customer data.

## Scenarios

### 1. Database restore failure

Inject an invalid SQL dump or stop the database service before import.

Expected result: the job transitions to `failed`, the audit log records the
failure, the failed counter increases, and the operator receives a clear
recovery instruction. The original site must remain intact.

### 2. Disk exhaustion during staging

Constrain the import filesystem and upload a backup larger than the available
space.

Expected result: the upload fails safely, temporary files are removed, no
partial archive is extracted into the target site, and the operator can retry
after freeing space.

### 3. Interrupted restore

Terminate the process during extraction and restart it.

Expected result: the incomplete job is visible in logs, the target is marked
for operator review, and a documented snapshot/rollback procedure is used
before retrying. A restore is never described as successful without a health
check.

## Incident workflow

1. Record impact, start time, affected site, and the job ID.
2. Preserve logs and the archive checksum before changing state.
3. Stop repeated retries if the same failure occurs twice.
4. Restore the last known-good snapshot or route traffic to the previous site.
5. Verify HTTP, PHP, database connectivity, permissions, and scheduled jobs.
6. Write a short postmortem with detection, timeline, root cause, and one
   prevention item.
