# StePanel

![CI](https://github.com/itchyitchy123/StePanel/actions/workflows/ci.yml/badge.svg) ![Release](https://img.shields.io/github/v/release/itchyitchy123/StePanel?display_name=tag) ![License](https://img.shields.io/github/license/itchyitchy123/StePanel)

> A modern, safety-first control plane for LAMP hosting and cPanel migrations.

StePanel is an open-source server management panel written in Go. It installs Apache, PHP, and a selectable MySQL or MariaDB version, provides a focused operations dashboard, and imports cPanel `cpmove` backups through an asynchronous, validated workflow.

It is designed for people who want a small, understandable hosting control plane instead of a large opaque platform.

## Why StePanel?

- **Migration-focused:** move cPanel accounts into a controlled LAMP environment.
- **Safety-first:** validate archives, reject unsafe entries, stage uploads privately, and expose restore status as a job.
- **Small footprint:** a Go control plane with a limited dependency surface.
- **Operator-friendly:** clear health endpoints, audit events, systemd deployment, and readable documentation.
- **Flexible database layer:** choose MySQL or MariaDB and request an exact repository version during installation.

## Current capabilities

| Area | Included today |
| --- | --- |
| Installation | Apache, PHP, MySQL/MariaDB, systemd, Debian/Ubuntu and RHEL-family systems |
| Migration | cPanel `.tar.gz` inspection, safe staging, website restore, optional SQL restore |
| Operations | Dashboard, health endpoint, metrics endpoint, audit log, asynchronous restore jobs |
| Security | bcrypt credentials, signed sessions, CSRF protection, archive traversal checks, restricted service user |
| Delivery | Dockerfile, ARM64/AMD64 release workflow, checksums, CI, vulnerability scanning |

> **Status:** StePanel is in early development. It is not yet a complete multi-tenant hosting platform. Run it behind authenticated HTTPS and test restores against a disposable server before using production data.

## See it quickly

![StePanel dashboard preview](docs/assets/dashboard-preview.svg)

See the [product preview](docs/SCREENSHOTS.md) for the current dashboard direction.

### Local development

Requirements: Go 1.22+.

```sh
git clone https://github.com/itchyitchy123/StePanel.git
cd StePanel
make check
go run .
```

Open <http://localhost:8080>. Local development uses `data/imports` and `data/www`, so root access is not required.

### Container

The container packages the control plane only. It does not run Apache or MySQL/MariaDB inside the container.

```sh
docker build -t stepanel:local .
docker run --rm -p 8080:8080 \
  -e STEPANEL_ADMIN_PASSWORD='change-me' \
  -e STEPANEL_SESSION_SECRET='use-at-least-32-random-characters' \
  stepanel:local
```

### Server installation

Build a release binary and run the installer as root:

```sh
go build -trimpath -ldflags='-s -w' -o stepanel .
sudo STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \
  STEPANEL_DB_ENGINE=mariadb \
  STEPANEL_DB_VERSION=default ./install.sh
```

The installer records the selected database engine/version, creates a restricted `stepanel` service account, and binds the control plane to `127.0.0.1:8090`. Configure [`deploy/apache/stepanel.conf`](deploy/apache/stepanel.conf) and HTTPS before exposing it.

## cpmove migration

1. Snapshot the destination server.
2. Open the migration center and upload a cPanel `.tar.gz` archive.
3. Choose the destination account and whether SQL should be restored.
4. Type `IMPORT` to authorize the operation.
5. Poll the returned job status until it completes or fails.
6. Review the audit log and verify the site before switching traffic.

Website files are restored to `/var/www/sites/<account>/public`. SQL dumps are restored to account-prefixed database names. Existing destination files can be overwritten; always snapshot first.

## Documentation

- [Installation guide](docs/INSTALLATION.md)
- [cpmove migration guide](docs/CPMOVE_IMPORTS.md)
- [Architecture and safety model](docs/ARCHITECTURE.md)
- [Operations runbook](docs/OPERATIONS.md)
- [Product roadmap](docs/ROADMAP.md)
- [Launch kit and repository metadata](docs/LAUNCH_KIT.md)
- [Portfolio direction](docs/PORTFOLIO.md)
- [Service objectives](docs/SLO.md)
- [Incident lab and recovery scenarios](docs/INCIDENT_LAB.md)
- [Observability bundle](observability/README.md)
- [Deployment examples](deploy/)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Release artifacts](https://github.com/itchyitchy123/StePanel/releases)

## API surface

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/api/health` | Version and service health |
| `POST` | `/api/cpmove/inspect` | Validate and inspect a backup |
| `POST` | `/api/cpmove/import` | Start an authorized restore job |
| `GET` | `/api/jobs/<id>` | Poll restore status |
| `GET` | `/metrics` | Prometheus-compatible process metric |

## Configuration

| Variable | Purpose |
| --- | --- |
| `STEPANEL_LISTEN` | HTTP listen address |
| `STEPANEL_ADMIN_USERNAME` | Administrator username |
| `STEPANEL_ADMIN_PASSWORD` | Administrator password |
| `STEPANEL_SESSION_SECRET` | Persistent session-signing secret, minimum 32 characters |
| `STEPANEL_DB_ENGINE` | `mysql` or `mariadb` during installation |
| `STEPANEL_DB_VERSION` | `default` or an exact repository version |
| `STEPANEL_IMPORT_ROOT` | Private backup staging directory |
| `STEPANEL_WEB_ROOT` | Site destination root |
| `STEPANEL_AUDIT_LOG` | JSONL audit log path |

## Roadmap

The next product milestones are first-run setup, site/domain lifecycle, PHP-FPM management, HTTPS automation, snapshot-backed rollback, scheduled backups, and multi-user roles. See the [roadmap](docs/ROADMAP.md) for the full plan.

## License

StePanel is released under the [MIT License](LICENSE).
