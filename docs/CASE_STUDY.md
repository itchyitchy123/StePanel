# Operational case study: interrupted restore drill

This is a reproducible failure drill, not a claim about an external customer
incident. It demonstrates the failure mode StePanel is designed to handle.

## Scenario

An administrator restores a cPanel archive while the site contains existing
files and a managed database. The process is terminated after the destination
has been prepared but before the restore commits.

## Expected behavior

1. The restore job is durable and reports an interrupted failure.
2. The recovery journal records the destination and any newly created database.
3. Startup recovery removes only transaction-owned database objects and rolls
   the site files back to the prior state.
4. Readiness remains failed if any recovery step cannot be completed.
5. A subsequent operator retry is safe and does not silently reuse the old
   database or activate an unverified archive.

## How to run it

Use the disposable lab in [`deploy/lab/README.md`](../deploy/lab/README.md),
run the restore workflow with synthetic data, terminate the panel process at a
documented transaction boundary, restart it, and verify the recovery journal,
database inventory, readiness endpoint, and audit events. Never run this drill
against production data.

## Evidence to retain

Keep the job record, recovery journal outcome, readiness response, database
inventory before/after, and the corresponding audit-event identifiers. This
turns recovery behavior into an observable operational result rather than a
design claim.
