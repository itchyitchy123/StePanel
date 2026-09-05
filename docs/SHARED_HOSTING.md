# Shared-hosting beta

StePanel can now provide constrained customer workspaces on one managed host.
This is a deliberate first boundary, not a claim of cPanel/Plesk parity.

## What is enforced

- An administrator creates a customer account with a `starter`,
  `professional`, or `agency` plan and explicit site assignments.
- Every customer account has an independent bcrypt password hash and a unique
  unpadded Base32 TOTP seed of at least 160 bits. Customer MFA is always
  required, including outside production mode.
- `starter`, `professional`, and `agency` cap assignments at 1, 5, and 25
  sites respectively. These are enforced assignment limits, not resource plans.
- Customer sessions can view only assigned site workspaces and their matching
  backup and job records. They can create a verified backup or add a domain
  route only for an assigned site.
- Cloud, SSH, service, database administration, migration, application,
  certificate, security, and account-management APIs remain administrator-only.

## Administrator API

`POST /api/accounts` is administrator-only and expects:

```json
{
  "username": "acme",
  "password": "a unique password with at least 20 characters",
  "totp_secret": "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
  "plan": "starter",
  "sites": ["acme-site"]
}
```

`GET /api/accounts` returns account metadata and plan limits, never password
hashes or TOTP seeds. Accounts expose a `suspended` lifecycle flag. Administrators
can suspend or unsuspend an account with `PATCH /api/accounts/{username}` and
`{"suspended":true|false}`; suspended customers cannot create new sessions.
`DELETE /api/accounts/{username}` terminates the account record and is audited.
Account data is stored in `STEPANEL_ACCOUNT_STATE`, mode
`0600`, and must be included in host backups. The production default is beside
the session state as `accounts.json`; configure a dedicated absolute path when
needed.

## Site environment variables

Set `STEPANEL_ENVIRONMENT_KEY` to enable encrypted site environment storage.
Use `GET`, `PUT`, and `DELETE /api/sites/environment/{site}` to inspect metadata,
replace variables, or remove them. Secret variables are encrypted at rest and
are returned only as metadata; values are never returned after they are written.
Back up the environment state file together with the encryption key.

## Signed Git webhooks

Set `STEPANEL_GIT_WEBHOOK_SECRET` to enable `POST /api/sites/git-webhook`.
Send `X-StePanel-Signature: sha256=<hex HMAC-SHA256>` with a JSON payload
containing `site`, `repository`, and an optional `ref`. Normal repository
allowlists and release validation still apply.

## Not implemented yet

Do not market this beta as unrestricted shared hosting. It does not yet provide
mailbox/FTP/SFTP/SSH lifecycle, browser file management, customer database
credentials, cron jobs, DNS/registrar lifecycle, usage accounting, bandwidth
or CPU/memory/disk quotas, billing, customer-initiated restores, support
workflows, reseller roles, or a multi-host control
plane. Those features require host-level enforcement and durable tenancy-aware
state, not merely additional dashboard forms.
