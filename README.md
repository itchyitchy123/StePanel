# StePanel

StePanel is a Go-based server control plane for small hosting fleets. It installs and supervises a LAMP stack, provides a focused operations dashboard, and can restore cPanel `cpmove` archives into isolated site roots.

## What is included

- Debian/Ubuntu and RHEL-family installation script for Apache, MySQL/MariaDB, PHP, and StePanel.
- systemd service with a restricted service account and writable paths limited to application data and web roots.
- cpmove `.tar.gz` inspection with archive size and path-traversal validation.
- staged website restore to `/var/www/sites/<account>/public`.
- optional SQL restore through the local `mysql` client, with account-prefixed database names.
- JSON health endpoint at `/api/health`.

## Install on a fresh server

Build the binary on a machine with Go 1.22+, copy the repository and binary to the server, then run:

```sh
go build -o stepanel .
sudo ./install.sh
```

The installer creates a `stepanel` system user, stores imports in `/var/lib/ste-panel/imports`, and binds the control plane to `127.0.0.1:8090`. Put Apache or an existing edge proxy in front of it and enable HTTPS before exposing the dashboard publicly.

## cpmove import

1. Open the StePanel dashboard.
2. Select a cPanel `.tar.gz`/`.tgz` archive and enter the destination account username.
3. Choose whether SQL databases should be restored.
4. Type `IMPORT` to explicitly authorize the restore.

The application validates the archive and extracts it into a timestamped staging directory before copying files. Database names are prefixed with the target account name. Existing site files may be overwritten, so take a snapshot before importing into a live site.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `STEPANEL_LISTEN` | `:8080` | HTTP listen address |
| `STEPANEL_IMPORT_ROOT` | `/var/lib/ste-panel/imports` | Staging directory |
| `STEPANEL_WEB_ROOT` | `/var/www` | Site destination root |

## Security notes

The initial release intentionally does not include user authentication or TLS termination. Deploy it behind an authenticated HTTPS reverse proxy or Apache before use on a network. Import endpoints require root and an explicit `IMPORT` confirmation, but authentication and role-based access are required before production exposure.
