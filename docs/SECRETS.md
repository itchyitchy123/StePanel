# StePanel secret lifecycle

StePanel stores several credentials outside the application binary. Treat the
files and keys below as control-plane secrets: keep them root/service-account
readable, include them in encrypted host backups, and never publish them with
site archives or support bundles.

## Environment encryption

Set `STEPANEL_ENVIRONMENT_KEY` to a stable 32-byte AES key encoded as base64.
`STEPANEL_ENVIRONMENT_STATE` contains encrypted per-site values. Back up both
the state file and the key together. A missing or changed key makes existing
secret values unrecoverable; it does not silently fall back to plaintext.

To rotate the key, export the current environment metadata and values through a
reviewed operator process, stop managed application updates, write the new key
to protected configuration, and rewrite each environment record. Verify a
decrypt/read test and service restart before retiring the old key. Do not
delete the old backup until recovery has been tested.

## Audit, session, and account state

`STEPANEL_AUDIT_KEY` protects the HMAC-linked audit chain. Do not rotate it in
place: existing events would no longer verify. Preserve the old key with the
audit log, and start a separately documented chain only through an intentional
key-rotation procedure.

`STEPANEL_SESSION_SECRET`, session state, and account state must be backed up
together when session recovery is required. Rotating the session secret is a
deliberate logout of existing sessions. If account or session state may have
been exposed, rotate administrator/customer credentials and revoke sessions.

## Git and off-site credentials

Per-site Git deploy keys are generated and retained by the privileged Git
helper; the panel API returns only their public half. Retire and regenerate a
key if its private file may have been copied. Review
`/etc/stepanel/git-known-hosts` when changing a provider host key; do not bypass
strict host verification.

Protect rclone/off-site configuration and provider credentials separately from
the panel state. Test an off-site restore without placing those credentials in
site environment variables or application logs.

## Recovery checklist

1. Keep an encrypted, access-controlled backup of the panel state, encryption
   keys, audit key, and documented file locations.
2. Test restoring environment state and verifying audit history on a disposable
   host after every key or packaging change.
3. If a key is lost, preserve the ciphertext for forensic purposes, record the
   loss in the audit/incident process, and recreate affected secrets rather
   than attempting plaintext recovery.
4. After recovery, rotate any credential that crossed the failed host boundary
   and confirm managed services use the newly rendered environment files.
