# cpmove imports

StePanel accepts gzip-compressed tar archives produced by cPanel, commonly named `cpmove-<account>.tar.gz`. The import form requires an account username and the literal confirmation `IMPORT`.

## Recommended procedure

1. Take a filesystem and database snapshot of the destination.
2. Upload the archive through an authenticated HTTPS connection.
3. Let the migration card inspect the archive, then confirm the detected
   website files, database dumps, mailbox count, and destination username.
4. Start the import and watch the job status until it is `completed` or
   `failed`.
5. Restore website files first and verify permissions and application configuration.
6. Restore SQL only when the database dump is trusted and compatible.
7. Review the staged archive and application logs before removing it.

Website files are copied to `/var/www/sites/<username>/public`. SQL files found
under the archive's `mysql` directory, including nested dump directories, are
restored to databases named `<username>_<database>`. If the dump name already
starts with the selected account prefix, StePanel preserves that database name
instead of duplicating the prefix. Existing files at the destination can be
overwritten.

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

The importer is intentionally conservative. It handles regular files and
directories inside `.tar.gz` archives, with or without a top-level
`cpmove-<account>/` or cPanel full-backup directory. Website files, SQL dumps,
and staged mailbox data are restored or preserved. Symlinks, device nodes,
unusual cPanel metadata, DNS zones, and account-level quotas are not restored
automatically.
