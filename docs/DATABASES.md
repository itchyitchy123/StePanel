# Database operations

StePanel installs and monitors one selected database engine. MySQL and MariaDB
support cPanel SQL and WordPress restores through the restricted database
helper. PostgreSQL is supported for installation, service monitoring,
connectivity checks, and phpPgAdmin access; StePanel does not translate MySQL
dumps into PostgreSQL syntax.

## Installation matrix

| Engine | Version selection | PHP extension | Browser administrator |
| --- | --- | --- | --- |
| MySQL | Distribution default or exact repository package version | `php-mysql`/`php-mysqlnd` | phpMyAdmin |
| MariaDB | Distribution default or exact repository package version | `php-mysql`/`php-mysqlnd` | phpMyAdmin |
| PostgreSQL | Distribution default; any available PostgreSQL AppStream stream on RHEL-family systems | `php-pgsql` | phpPgAdmin |

Example RHEL-family PostgreSQL installation:

```sh
dnf module list postgresql --all
sudo STEPANEL_DB_ENGINE=postgresql \
  STEPANEL_DB_VERSION=16 \
  STEPANEL_INSTALL_DB_ADMIN=1 \
  STEPANEL_DB_ADMIN_ALLOW='127.0.0.1 ::1 10.20.0.0/16' \
  STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \
  STEPANEL_PANEL_HOSTNAME=panel.example.com \
  ./install.sh
```

The explicit stream must appear in the `dnf module list` output. The installer
enables the selected stream and installs the unversioned `postgresql-server`
RPM from that stream.

## Administration UI security

`STEPANEL_INSTALL_DB_ADMIN=1` installs phpMyAdmin for MySQL/MariaDB or
phpPgAdmin for PostgreSQL. On Apache, StePanel writes a dedicated Alias and
Directory policy. Access defaults to loopback only and is expanded solely by
`STEPANEL_DB_ADMIN_ALLOW`. Values must be literal IP addresses or CIDR
networks; hostnames and arbitrary Apache directives are rejected.

The database administrator has its own authentication boundary. A StePanel
session does not sign a user into phpMyAdmin or phpPgAdmin. Keep the route
behind HTTPS and a VPN or management network. For Caddy and OpenLiteSpeed, the
package can be installed but the PHP route must be reviewed and configured by
the operator before the dashboard reports it ready.

## Verification and troubleshooting

Use the authenticated `/api/database` endpoint to see the selected engine,
configured version, service state, detected client, and admin-console state.
The authenticated `/api/security/audit` endpoint also warns when an installed
console lacks the StePanel Apache policy or when its policy grants access to
all clients.
On the host, verify the corresponding components:

```sh
systemctl status mysql mariadb postgresql php-fpm --no-pager
php -m | grep -E 'mysqli|pdo_mysql|pgsql|pdo_pgsql'
apachectl -t
```

Debian/Ubuntu may use a versioned PHP-FPM unit such as `php8.3-fpm`; StePanel
detects these units. If the database administrator is installed but marked
unavailable, check `/etc/apache2/stepanel-panel/db-admin.conf` or
`/etc/httpd/stepanel-panel/db-admin.conf`, confirm the application directory
under `/usr/share`, and review the configured IP allowlist.

For a remote database, set `STEPANEL_DB_HOST`, `STEPANEL_DB_USER`, and
`STEPANEL_DB_PASSWORD` together. Installation performs a non-interactive
connectivity check with the selected engine's client. Configure the remote
server separately in phpMyAdmin/phpPgAdmin if browser administration should
target it; StePanel does not write remote credentials into those applications.
