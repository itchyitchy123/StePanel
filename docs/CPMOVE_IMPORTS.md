# cpmove imports

StePanel accepts gzip-compressed tar archives produced by cPanel, commonly named `cpmove-<account>.tar.gz`. The import form requires an account username and the literal confirmation `IMPORT`.

## Recommended procedure

1. Take a filesystem and database snapshot of the destination.
2. Upload the archive through an authenticated HTTPS connection.
3. Confirm the detected archive contents and destination username.
4. Restore website files first and verify permissions and application configuration.
5. Restore SQL only when the database dump is trusted and compatible.
6. Review the staged archive and application logs before removing it.

Website files are copied to `/var/www/sites/<username>/public`. SQL files found under the archive's `mysql` directory are restored to databases named `<username>_<database>`. Existing files at the destination can be overwritten.

## Compatibility

The importer is intentionally conservative. It currently handles regular files and directories inside `.tar.gz` archives. Symlinks, device nodes, unusual cPanel metadata, mailboxes, DNS zones, and account-level quotas are not restored automatically.
