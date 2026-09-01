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
- Login attempts are rate-limited.
- Uploads are size-limited and staged privately.
- Archive paths are checked for absolute paths and traversal.
- Restore destinations are account-scoped and SQL restoration is opt-in.
- Audit events are appended to a mode-0600 JSONL file.
- Active Apache snippets are root-owned and rendered by a fixed-template helper.
- Concurrent restores targeting the same site are rejected.
- New database destinations are rolled back on restore failure; existing
  databases and database users are never silently reused.

## Residual risks

- A compromised administrator can request destructive restores.
- A restored WordPress application is untrusted code after extraction.
- The service account holds a powerful database administration credential;
  compromise of the control plane therefore requires credential rotation and
  database integrity review.
- TLS and MFA/OIDC remain deployment responsibilities or roadmap items.

## Operator requirements

Run StePanel behind HTTPS, restrict the listener to a private interface or
reverse proxy, snapshot the destination before restores, and review the audit
log after every import.
