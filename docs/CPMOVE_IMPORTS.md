# cpmove imports

StePanel accepts gzip-compressed tar archives produced by cPanel, commonly named `cpmove-<account>.tar.gz`. The import form requires an account username and the literal confirmation `IMPORT`.

## Recommended procedure

1. Take a filesystem and database snapshot of the destination.
2. Upload the archive through an authenticated HTTPS connection.
3. Confirm the detected archive contents and destination username.
4. Poll the returned job status until it is `completed` or `failed`.
5. Restore website files first and verify permissions and application configuration.
6. Restore SQL only when the database dump is trusted and compatible.
7. Review the staged archive and application logs before removing it.

Website files are copied to `/var/www/sites/<username>/public`. SQL files found under the archive's `mysql` directory are restored to databases named `<username>_<database>`. Existing files at the destination can be overwritten.

## Mail restoration

When `STEPANEL_INSTALL_MAIL=1` is used, the installer installs Exim, Dovecot,
and SpamAssassin and creates a private mail staging root. cPanel mailbox data found
under `homedir/mail` is preserved under:

```text
/var/lib/ste-panel/mail/<account>/mail/
/var/lib/ste-panel/mail/<account>/etc/
```

The restore result reports staged mailboxes. cPanel's Exim and Dovecot files
are host-specific and are not copied into `/etc`; activating mail requires a
domain/mailbox mapping step, credentials, DNS records, TLS, and transport
policy on the destination.

SpamAssassin is installed and its daemon is started for mail installations, but
the destination administrator must still connect it to the chosen Exim
transport and review scoring, relay, and training policy before accepting mail.

## Compatibility

The importer is intentionally conservative. It currently handles regular files
and directories inside `.tar.gz` archives, website files, SQL dumps, and staged
mailbox data. Symlinks, device nodes, unusual cPanel metadata, DNS zones, and
account-level quotas are not restored automatically.
