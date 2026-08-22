# StePanel

![CI](https://github.com/itchyitchy123/StePanel/actions/workflows/ci.yml/badge.svg)

StePanel is a focused server control plane for small hosting fleets. It installs a LAMP foundation, exposes a calm operational dashboard, and provides a guarded workflow for importing cPanel `cpmove` backups.

> **Project status:** early development. Authentication, authorization, TLS termination, and a complete multi-tenant control plane are not implemented yet. Use behind an authenticated HTTPS reverse proxy and review every restore in a non-production environment first.

## Capabilities

- Apache, MySQL/MariaDB, PHP, and StePanel installation on Debian/Ubuntu and RHEL-family servers.
- Dedicated system user and systemd service with restricted writable paths.
- cpmove `.tar.gz` validation before extraction, including archive size and traversal checks.
- Timestamped private staging for backups.
- Website restore to `/var/www/sites/<account>/public`.
- Optional SQL restore using account-prefixed database names.
- JSON health endpoint for monitoring integrations.
- Server-rendered Go application with a small dependency surface.

## Quick start

### Build and run locally

Requires Go 1.22 or newer.

```sh
make check
go run .
```

Open <http://localhost:8080>. Local development defaults to `data/imports` and `data/www`, so root access is not required.

### Install on a server

Build a release binary, copy the repository and binary to the target host, and run the installer as root:

```sh
go build -trimpath -ldflags='-s -w' -o stepanel .
sudo ./install.sh
```

The installer creates a `stepanel` service account, installs the LAMP packages, configures systemd, and binds StePanel to `127.0.0.1:8090`. Configure the Apache reverse proxy in [`deploy/apache/stepanel.conf`](deploy/apache/stepanel.conf) and enable HTTPS before exposing it.

## Documentation

- [Installation guide](docs/INSTALLATION.md)
- [cpmove import guide](docs/CPMOVE_IMPORTS.md)
- [Architecture and safety model](docs/ARCHITECTURE.md)
- [Operations runbook](docs/OPERATIONS.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)

## Configuration

| Variable | Default in development | Installed default | Purpose |
| --- | --- | --- | --- |
| `STEPANEL_LISTEN` | `:8080` | `127.0.0.1:8090` | HTTP listen address |
| `STEPANEL_IMPORT_ROOT` | `data/imports` | `/var/lib/ste-panel/imports` | Private staging directory |
| `STEPANEL_WEB_ROOT` | `data/www` | `/var/www` | Site destination root |
| `STEPANEL_AUDIT_LOG` | `data/stepanel-audit.jsonl` | `/var/lib/ste-panel/audit.jsonl` | Append-only operational audit log |

Set `STEPANEL_ADMIN_USERNAME`, `STEPANEL_ADMIN_PASSWORD`, and a long random `STEPANEL_SESSION_SECRET` in `/etc/ste-panel.env` before exposing the service. Authentication is disabled when the admin password is absent, which is intended only for local development.

## API surface

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/api/health` | Service discovery and liveness |
| `POST` | `/api/cpmove/inspect` | Validate and inspect a multipart backup |
| `POST` | `/api/cpmove/import` | Authorized restore of files and optional SQL |
| `GET` | `/metrics` | Minimal Prometheus-compatible process metric |

## Security model

The importer requires root, rejects unsafe archive paths, stages the upload privately, and requires the literal `IMPORT` confirmation. SQL restoration is opt-in. Existing site files can be overwritten, so snapshot the destination first.

StePanel does not yet provide authentication or TLS. Do not bind it publicly. Place it behind an authenticated reverse proxy, restrict access to trusted administrators, and use HTTPS.

## License

StePanel is released under the [MIT License](LICENSE).
