# Security Policy

## Supported versions

Until the first stable release, security fixes are applied to the `main` branch. Production operators should pin a reviewed commit rather than track an unreviewed branch.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Contact the repository owner privately through GitHub with:

- affected commit or version;
- reproduction steps or a minimal proof of concept;
- impact and likely attack prerequisites;
- any suggested mitigation.

Please allow time for investigation and a coordinated fix before public disclosure.

## Deployment requirements

StePanel provides administrator authentication, but production deployments
must still run behind HTTPS and a reverse proxy. Do not expose port 8080/8090
directly to the internet. Restrict backup staging permissions and use
snapshots before restoring into a live site. Production startup requires TOTP
MFA and an enforced offsite backup target; OIDC, role separation, and a
customer-scoped privileged restore workflow remain roadmap items.

Privileged helpers use context-bound commands with bounded output. Cloud CLI
children receive provider credentials and region settings but not panel-specific
session, audit, or database secrets. Keep provider credential files and rclone
configuration outside site-controlled paths and restrict them to the service
account or root.

Malformed recovery journals are moved into a root-only quarantine directory for
operator review. Do not delete quarantined journals until the related site,
database, and audit state has been reconciled.
