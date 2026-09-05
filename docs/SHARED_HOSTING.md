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
hashes or TOTP seeds. Account data is stored in `STEPANEL_ACCOUNT_STATE`, mode
`0600`, and must be included in host backups. The production default is beside
the session state as `accounts.json`; configure a dedicated absolute path when
needed.

## Not implemented yet

Do not market this beta as unrestricted shared hosting. It does not yet provide
mailbox/FTP/SFTP/SSH lifecycle, browser file management, customer database
credentials, cron jobs, DNS/registrar lifecycle, usage accounting, bandwidth
or CPU/memory/disk quotas, billing, customer-initiated restores, support
workflows, reseller roles, account suspension/deletion, or a multi-host control
plane. Those features require host-level enforcement and durable tenancy-aware
state, not merely additional dashboard forms.
