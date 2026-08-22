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

StePanel must run behind HTTPS and an authenticated reverse proxy until native authentication is implemented. Do not expose port 8080/8090 directly to the internet. Restrict backup staging permissions and use snapshots before restoring into a live site.
