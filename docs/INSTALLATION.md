# Installation guide

## Supported operating systems

The installer supports systems with `apt-get` (Debian/Ubuntu) or `dnf` (Fedora, Rocky, Alma, and compatible RHEL-family systems). It installs Apache, MySQL/MariaDB, PHP, archive utilities, and StePanel.

## Build and install

Build on a Go 1.22+ build host:

```sh
go build -trimpath -ldflags='-s -w' -o stepanel .
sudo STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \\
  STEPANEL_DB_ENGINE=mariadb \\
  STEPANEL_DB_VERSION=10.11 ./install.sh
```

The installer requires an admin password, generates a session secret when one is not supplied, and starts in production mode. It supports `mysql` and `mariadb`. Use `default` for the distribution-provided version, or provide an exact version available from the configured package repositories. The installer verifies requested versions before changing the system and fails rather than silently selecting another version.

In an interactive terminal, the installer asks for the database engine and version. For automation, use:

| Variable | Values | Meaning |
| --- | --- | --- |
| `STEPANEL_DB_ENGINE` | `mysql`, `mariadb` | Database distribution |
| `STEPANEL_DB_VERSION` | `default` or an exact package version | Requested repository version |
| `STEPANEL_INSTALL_MAIL` | `0` or `1` | Install Exim, Dovecot, SpamAssassin, and enable mailbox staging |
| `STEPANEL_INSTALL_NODE` | `0` or `1` | Install NVM for the StePanel service account |
| `STEPANEL_NODE_VERSIONS` | Comma-separated versions | Node versions to install through NVM |
| `STEPANEL_APP_ROOT` | Directory | JSON manifests for managed Node apps |
| `STEPANEL_INSTALL_SECURITY` | `0` or `1` | Install ClamAV and the PHP malware guard |
| `STEPANEL_INSTALL_TLS` | `0` or `1` | Install Certbot and the Apache certificate integration |

It also enables the Apache proxy, proxy_http, and headers modules on Debian-family systems. Replace the example hostname in the generated virtual host before production use. The installer creates:

| Path | Purpose |
| --- | --- |
| `/opt/stepanel` | Binary and web assets |
| `/var/lib/ste-panel/imports` | Private cpmove staging |
| `/var/lib/ste-panel/mail` | Private staged mailbox data |
| `/var/lib/ste-panel/proxy` | Managed Apache reverse-proxy snippets |
| `/var/lib/ste-panel/apps` | Managed Node application manifests |
| `/var/lib/ste-panel/quarantine` | Recoverable malware quarantine |
| `/etc/ste-panel.env` | Runtime configuration |
| `/etc/systemd/system/stepanel.service` | Service definition |

The installer enables TLS issuance only when `STEPANEL_INSTALL_TLS=1`. Metrics
are authenticated by default; set `STEPANEL_METRICS_PUBLIC=1` only on a
protected monitoring network.

## Reverse proxy

Copy `deploy/apache/stepanel.conf` to the Apache configuration directory, replace the example hostname, enable the required proxy modules, and reload Apache. Add TLS with your preferred certificate automation before exposing the host.

## Operations

```sh
systemctl status stepanel
journalctl -u stepanel -f
curl http://127.0.0.1:8090/api/health
```

## Uninstall

Stop and disable the service, remove `/opt/stepanel`, and decide separately whether staged imports under `/var/lib/ste-panel` should be retained for audit or deleted after verification.
