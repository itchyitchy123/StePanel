# WordPress WPress imports

StePanel incorporates the standalone WPress Restore workflow from the sibling
`wpress_restore` project and adapts it to StePanel site roots and database
configuration.

## Host prerequisites

Install these commands on the destination host:

- `wpress-extract` from the WPress extractor package;
- WP-CLI (`wp`);
- `mariadb` or `mysql`.

Override executable paths with `STEPANEL_WPRESS_EXTRACT` and `STEPANEL_WPCLI`.
Use `/api/wpress/preflight` to verify availability.

## Restore procedure

1. Take a filesystem and database snapshot.
2. Open the WordPress migration card in the authenticated dashboard.
3. Select the `.wpress` archive and a site account.
4. Enter database and database-user suffixes, plus a new database password.
5. Optionally enter the destination URL and table prefix.
6. Enable overwrite only after verifying the backup.
7. Type `WPRESTORE` and wait for the job to complete.
8. Verify WordPress, permalinks, plugins, uploads, and database connectivity.

The resulting names are prefixed with the site account, for example
`example_wordpress` and `example_wpuser`. The restore creates the database and
local database user through the configured MySQL/MariaDB administrative
connection, imports `database.sql`, and uses WP-CLI for serialized-safe table
prefix and URL replacement.

Archives are extracted into a private staging directory. Symlinks are rejected,
the database dump is not copied into the public site, and a failed overwrite
attempt restores the previous site directory. WordPress code and plugins are
untrusted after extraction; scan and update them before placing the site on
public traffic.
