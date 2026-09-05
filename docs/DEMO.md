# StePanel demo checklist

Use a disposable Linux VM or container for demonstrations. Do not use a
customer server or real backup archive.

## Suggested seven-minute walkthrough

1. Show the version and selected MySQL/MariaDB/PostgreSQL installation options.
2. Open the authenticated dashboard and health endpoint.
3. Open a managed-site workspace and show grouped domains, applications,
   proxies, databases, and verified activity.
4. Create a disposable customer account with one assigned demonstration site
   and a freshly generated TOTP seed. Sign in as that account, show its plan
   and site summary, then verify that an administrator-only endpoint such as
   `/api/services` is denied.
5. Deploy a pre-built public repository ref, show the exact commit, and
   demonstrate atomic Git rollback on a disposable site.
6. Inspect credential-safe database detail and the diagnostics endpoint as an
   administrator.
7. Inspect a synthetic cpmove archive, start an import, and poll its job ID.
8. Open `/metrics` and the Grafana dashboard to show restore counters as an
   administrator.
9. Stop the database service, repeat the import, and show the failed-job and
   incident workflow from [`INCIDENT_LAB.md`](INCIDENT_LAB.md).

Record only synthetic usernames, domains, and data. Include the commit SHA in
the recording description so the demo is reproducible.

## Evidence to publish

- A short screen recording or terminal capture
- Test archive checksum
- Host distribution and resource size
- Restore duration and result
- Customer-scope isolation result, including the denied administrator endpoint
- Relevant test command and commit SHA
