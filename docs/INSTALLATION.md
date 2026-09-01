# Installation guide

## Supported operating systems

The installer supports systems with `apt-get` (Debian/Ubuntu) or `dnf` (Fedora, Rocky, Alma, and compatible RHEL-family systems). It installs Apache, MySQL/MariaDB, PHP-FPM, ACL and archive utilities, and StePanel.

## Build and install

Build on a Go 1.26+ build host:

```sh
go build -trimpath -ldflags='-s -w' -o stepanel .
sudo STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \\
  STEPANEL_PANEL_HOSTNAME=panel.example.com \\
  STEPANEL_DB_ENGINE=mariadb \\
  STEPANEL_DB_VERSION=10.11 ./install.sh
```

For a fresh installation, the installer requires an admin password, stores only
its bcrypt hash, generates a session secret when one is not supplied, and starts in production mode. It
supports `mysql` and `mariadb`. Use `default` for the distribution-provided
version, or provide an exact version available from the configured package
repositories. The installer verifies requested versions before installing the
database package and fails rather than silently selecting another version. For a local
database it installs a root-owned restore helper and does not place a global
database credential in the control-plane environment. The helper creates only
validated new schemas, imports through a temporary account restricted to that
schema, and records managed objects for cleanup. Set `STEPANEL_DB_HOST`,
`STEPANEL_DB_USER`, and `STEPANEL_DB_PASSWORD` together when using a
pre-provisioned remote database; remote credential scope remains the operator's
responsibility.
When upgrading an older installation, verify that `STEPANEL_DBCTL` is active
and restores succeed before manually removing the legacy `stepanel_admin`
database account; the installer does not delete an existing account implicitly.
It also requires the panel's fully qualified hostname and writes it into the
Apache virtual host. Production sessions use Secure cookies, so complete TLS
termination before attempting to sign in. When Certbot integration is
installed, issuance can be bootstrapped from the host with
`sudo stepanel-certbot panel.example.com admin@example.com`.

## Upgrades and rollback boundary

Running the installer over an existing installation preserves values from the
root-owned, non-writable `/etc/ste-panel.env` unless the corresponding
`STEPANEL_*` variable is explicitly supplied. This preserves the administrator
hash, session secret, database selection and credentials, hostname, and runtime
paths; an upgrade therefore does not require the plaintext administrator
password.

Before replacing StePanel-owned files, the installer snapshots the binary, web
assets, helpers, environment, sudoers policy, systemd unit, logrotate policy,
and panel Apache configuration into a private `/var/tmp/stepanel-install.*`
transaction. It stops the old daemon, installs the candidate, validates the
complete Apache configuration, starts the candidate, and requires the local
health endpoint to succeed. A failure restores those files and the prior
StePanel service state. Apache changes share the same lock as vhost, proxy, and
certificate operations.

Package-manager transactions and optional third-party mail, FTP, ModSecurity,
Fail2ban, Node, ClamAV, or Certbot package configuration are outside that file
rollback boundary. Snapshot the host and verified backup artifacts before an
upgrade, and review package-manager history if dependency installation fails.

## Administrator MFA and audit key

Set `STEPANEL_ADMIN_TOTP_SECRET` to an unpadded base32 secret representing at
least 20 random bytes to require a six-digit authenticator code at every login.
Generate a secret on the host and enroll the same value manually in the
operator's authenticator before running the installer:

```sh
umask 077
totp_secret=$(head -c 20 /dev/urandom | base32 | tr -d '=\n')
sudo STEPANEL_ADMIN_TOTP_SECRET="$totp_secret" ./install.sh
```

Test a fresh login before closing the recovery console. If the authenticator is
lost, a local root operator can rerun the installer with
`STEPANEL_ADMIN_TOTP_SECRET=''` to remove the requirement. StePanel remains a
single-administrator system; teams should not share the account when individual
attribution is required.

The installer generates a separate audit HMAC key, stores a root-only copy at
`/etc/stepanel-audit.key`, and supplies it through the protected service
environment. Preserve both in the host's secret backup.

FTP is opt-in. Installation alone leaves a newly installed vsftpd service
disabled. Activation requires `STEPANEL_ACTIVATE_FTP=1` and readable certificate
and private-key paths; the resulting configuration requires TLS for both login
and data channels. The installer does not create FTP accounts. Mail packages
are likewise installed without activating newly installed daemons unless
`STEPANEL_ACTIVATE_MAIL=1` is explicit. Configure domains, TLS, DNS, anti-relay
rules, firewall policy, and monitoring before activating mail.

In an interactive terminal, the installer asks for the database engine and version. For automation, use:

| Variable | Values | Meaning |
| --- | --- | --- |
| `STEPANEL_DB_ENGINE` | `mysql`, `mariadb` | Database distribution |
| `STEPANEL_ADMIN_TOTP_SECRET` | Unpadded base32, 160 bits or more | Require TOTP MFA for administrator login |
| `STEPANEL_PANEL_HOSTNAME` | Fully qualified domain | Required Apache virtual-host name |
| `STEPANEL_DB_VERSION` | `default` or an exact package version | Requested repository version |
| `STEPANEL_INSTALL_MAIL` | `0` or `1` | Install Exim, Dovecot, SpamAssassin, and enable mailbox staging |
| `STEPANEL_ACTIVATE_MAIL` | `0` or `1` | Explicitly enable/start the installed mail services |
| `STEPANEL_INSTALL_FTP` | `0` or `1` | Install vsftpd with local-user chrooting; newly installed service remains disabled |
| `STEPANEL_ACTIVATE_FTP` | `0` or `1` | Require FTPS configuration and enable/start vsftpd |
| `STEPANEL_FTP_CERT_FILE` / `STEPANEL_FTP_KEY_FILE` | Absolute paths | Required certificate and key when activating FTPS |
| `STEPANEL_FTP_PASSIVE_MIN` / `STEPANEL_FTP_PASSIVE_MAX` | Port range | Passive FTP port range; defaults to `40100-40200` |
| `STEPANEL_INSTALL_NODE` | `0` or `1` | Install NVM for the StePanel service account |
| `STEPANEL_NODE_VERSIONS` | Comma-separated versions | Node versions to install through NVM |
| `STEPANEL_INSTALL_SECURITY` | `0` or `1` | Install ClamAV and the PHP malware guard |
| `STEPANEL_INSTALL_TLS` | `0` or `1` | Install Certbot and the Apache certificate integration |
| `STEPANEL_STAGE_RETENTION_HOURS` | Positive hours | Retain completed restore staging directories for audit |
| `STEPANEL_MIN_FREE_BYTES` | Bytes | Refuse new restores below this free-space threshold |
| `STEPANEL_MAX_UPLOAD_BYTES` | Bytes, up to 20 GiB | Maximum compressed restore request size |
| `STEPANEL_MAX_ARCHIVE_ENTRIES` | `1`–`1000000` | Maximum filesystem entries in a restore or backup |
| `STEPANEL_MAX_CONCURRENT_JOBS` | `1`–`32` | Global restore, backup, and certificate job slots |

It also enables the Apache proxy, proxy_http, proxy_fcgi, setenvif, rewrite, and headers modules on
Debian-family systems and writes the requested hostname into the generated
virtual host. The installer creates:

On SELinux systems the installer restores the standard file contexts and
enables `httpd_can_network_connect`, which is required for Apache reverse
proxying. Review that boolean against the server's PHP execution model and
outbound-network policy.

| Path | Purpose |
| --- | --- |
| `/opt/stepanel` | Binary and web assets |
| `/var/lib/ste-panel/imports` | Private cpmove staging |
| `/var/backups/stepanel` | Private, verified site backup artifacts |
| `/var/lib/ste-panel/mail` | Private staged mailbox data |
| `/etc/apache2/stepanel-proxy` or `/etc/httpd/conf.d/stepanel-proxy` | Root-owned managed Apache reverse-proxy snippets |
| `/etc/apache2/stepanel-sites` or `/etc/httpd/conf.d/stepanel-sites` | Root-owned managed PHP site vhosts |
| `/var/lib/ste-panel/apps` | Managed Node application manifests |
| `/var/lib/ste-panel/quarantine` | Recoverable malware quarantine |
| `/var/www/sites/.stepanel-recovery` | Journaled site rollback data |
| `/etc/ste-panel.env` | Runtime configuration |
| `/etc/stepanel-audit.key` | Root-only HMAC key for audit verification |
| `/etc/systemd/system/stepanel.service` | Service definition |
| `/etc/logrotate.d/stepanel` | Audit-log retention policy |

The installer enables TLS issuance only when `STEPANEL_INSTALL_TLS=1`. Metrics
are authenticated by default; set `STEPANEL_METRICS_PUBLIC=1` only on a
protected monitoring network.

## Site isolation

Before a host restore writes a document root, the root-owned site helper creates
a deterministic system user and unique primary group, establishes inherited
ACL access for the unprivileged control plane, and renders private PHP-FPM
pools for installed PHP versions. Restored files are sealed to the site user
and Apache group afterward. Site users are not added to Apache's shared group;
Apache receives group read access to static files and PHP-FPM sockets instead.
The site deployment API renders a validated vhost that routes PHP requests to
an active pool for that site and refuses domains already claimed by an existing
Apache vhost or managed Node proxy.

## Reverse proxy

Copy `deploy/apache/stepanel.conf` to the Apache configuration directory, replace the example hostname, enable the required proxy modules, and reload Apache. Add TLS with your preferred certificate automation before exposing the host.

## Operations

```sh
systemctl status stepanel
journalctl -u stepanel -f
curl -i http://127.0.0.1:8090/livez
curl -fsS http://127.0.0.1:8090/readyz | jq
```

## Uninstall

Stop and disable the service, remove `/opt/stepanel`, and decide separately whether staged imports under `/var/lib/ste-panel` should be retained for audit or deleted after verification.
