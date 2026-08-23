# Installation guide

## Supported operating systems

The installer supports systems with `apt-get` (Debian/Ubuntu) or `dnf` (Fedora, Rocky, Alma, and compatible RHEL-family systems). It installs Apache, MySQL/MariaDB, PHP, archive utilities, and StePanel.

## Build and install

Build on a Go 1.22+ build host:

```sh
go build -trimpath -ldflags='-s -w' -o stepanel .
sudo STEPANEL_ADMIN_PASSWORD='use-a-password-manager' ./install.sh
```

The installer requires an admin password, generates a session secret when one is not supplied, and starts in production mode. It also enables the Apache proxy, proxy_http, and headers modules on Debian-family systems. Replace the example hostname in the generated virtual host before production use. The installer creates:

| Path | Purpose |
| --- | --- |
| `/opt/stepanel` | Binary and web assets |
| `/var/lib/ste-panel/imports` | Private cpmove staging |
| `/etc/ste-panel.env` | Runtime configuration |
| `/etc/systemd/system/stepanel.service` | Service definition |

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
