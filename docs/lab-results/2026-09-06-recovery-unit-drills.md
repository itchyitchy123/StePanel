# Recovery drill results — 2026-09-06

This record contains measured results from the repository's disposable,
filesystem-backed recovery tests. It is not a production incident report and
does not replace the Docker/systemd lab or a real migration exercise.

## Environment

- Host: Linux development workspace
- Revision: working tree under review on 2026-09-06
- Test command: `GOCACHE=/tmp/stepanel-go-cache GOMAXPROCS=1 go test -run <pattern> -count=1 ./`
- Data: test-created temporary directories and synthetic files/databases

## Results

| Scenario | Test evidence | Result | Elapsed |
| --- | --- | --- | ---: |
| Partial SQL import is rejected and cleaned up | `TestRestoreSQLDropsPartiallyImportedDatabase`; `TestRestoreFailureRestoresExistingSite` | PASS | 0.081s |
| Interrupted/recovery journals restore site state and clean transaction-owned data | `TestRecoverTransactionDatabases`; `TestRecoverSiteTransactions`; `TestSiteTransactionRollback` | PASS | 0.247s |
| Failed web configuration mutation restores the prior configuration | `TestGitRollbackAtomicallySwapsPreviousRelease`; `TestProxyDeleteRestoresConfigWhenReloadFails` | PASS | 0.030s |
| Runtime reconciliation retains pending state after helper failure | `TestReconcileWorkersRetainsPendingStateWhenHelperFails`; `TestReconcilePythonAppsRetainsPendingStateWhenHelperFails`; `TestReconcilePHPProfilesRetainsPendingStateWhenHelperFails` | PASS | 0.061s |

## Interpretation

The tests demonstrate local rollback and pending-state behavior with synthetic
inputs. They do not prove recovery from power loss, real database-server
failure, filesystem exhaustion, systemd reload failure on a target OS, or
off-site transfer interruption. Those scenarios require the disposable
systemd/Docker lab described in [`deploy/lab/README.md`](../../deploy/lab/README.md).

## Follow-up

- Run the Docker/systemd scenarios on a host with Docker available.
- Capture the tagged revision, failure-injection command, logs, readiness
  response, audit identifiers, and filesystem/database state before and after.
- Add the resulting evidence here without replacing these repository-test
  results with unmeasured claims.
