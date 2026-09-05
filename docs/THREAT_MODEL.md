# Threat model

## Assets

- Administrator credentials and session cookies.
- Uploaded cPanel backups and extracted site files.
- Database credentials and imported SQL data.
- Server configuration and audit history.

## Trust boundaries

1. The browser is untrusted and can submit arbitrary form fields.
2. Uploaded archives are untrusted data and may contain malicious paths or files.
3. StePanel runs as a service account and must not become a general root command
   execution interface.
4. Apache, PHP, and database services are external system dependencies.

## Existing controls

- Authenticated mutating routes require CSRF validation.
- Sessions are signed, expiring, and protected with HttpOnly/SameSite cookies.
- Optional TOTP adds replay-resistant second-factor validation; accepted codes
  cannot be reused within the process lifetime.
- Login attempts are rate-limited.
- Uploads are size-limited and staged privately.
- Archive paths are checked for absolute paths and traversal.
- Restore destinations are account-scoped and SQL restoration is opt-in.
- Audit events distinguish actor from target, are sequence/HMAC linked, and
  fail closed before authenticated mutating handlers when persistence is down.
- Active Apache snippets are root-owned and rendered by a fixed-template helper.
- Concurrent restores targeting the same site are rejected.
- New database destinations are rolled back on restore failure; existing
  databases and database users are never silently reused.
- Background job state and site-overwrite transactions are persisted before
  mutations begin; interrupted site transactions are rolled back at startup.
- Host site workloads use deterministic per-site Unix identities and unique
  primary groups. Apache receives group access without making site users
  members of its shared group, and the control plane uses explicit ACLs.
- Git deployment accepts only allowlisted HTTPS hosts, disables credential
  prompts/helpers, rejects symlinks and special files, strips repository
  metadata, and does not execute repository-provided build scripts.

## Residual risks

- A compromised administrator can request destructive restores.
- A restored WordPress application is untrusted code after extraction.
- Remote database deployments may give the service account a powerful
  administration credential; compromise requires credential rotation and a
  database integrity review. Local installs instead use the restricted helper.
- TLS and multi-user OIDC remain deployment responsibilities or roadmap items;
  the built-in administrator is still a single shared identity.
- Backup checksums detect corruption but are not signatures. An actor able to
  modify both an archive and its manifest can replace both.
- An allowlisted Git provider and repository contents remain trusted inputs.
  Compromise of either can publish malicious application code even though the
  deployment path prevents control-plane command execution.

## Operator requirements

Run StePanel behind HTTPS, restrict the listener to a private interface or
reverse proxy, snapshot the destination before restores, and review the audit
log after every import. Replicate completed backup directories to independently
controlled immutable or off-host storage and verify them again there.
