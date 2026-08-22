# StePanel

StePanel is an original infrastructure operations cockpit written in Go. It is being designed as a calm, high-signal control plane for small fleets: health at a glance, resource activity, maintenance windows, and an extensible API for future provisioning and observability features.

This repository is intentionally independent of the referenced `zepanel` project. That repository currently contains only a GitLab scaffold and has no application implementation to port.

## Run locally

Requires Go 1.22 or newer.

```sh
go run .
```

Open <http://localhost:8080>.

## Direction

- Go standard-library backend first, with a small dependency surface.
- Server-rendered HTML for the initial cockpit and progressively enhanced interactions.
- Clear separation between control-plane API, resource adapters, and presentation.
- Distinct visual identity: warm paper canvas, dark ink, lime signal color, and operational typography.

## Planned modules

The next implementation slice is authentication and a real resource model, followed by adapters for system metrics, service lifecycle, backups, certificates, and audit events.
