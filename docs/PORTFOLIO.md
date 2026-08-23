# StePanel product portfolio

StePanel is the reference implementation for a small, self-hosted hosting
platform. The surrounding materials define three credible engineering
projects that can be split into independent repositories as they mature:

## SteObserve

An opinionated Prometheus/Grafana observability layer for PHP-FPM and Linux
hosting. The first slice is in [`../observability`](../observability):
low-cardinality application metrics, a dashboard, scrape configuration, and
service objectives.

## StePlatform

A platform-engineering reference stack: the Helm chart and Kubernetes
manifests in [`../deploy`](../deploy) show how to package the control plane,
configure secrets, expose health probes, and promote immutable images through
environments. The examples are intentionally provider-neutral.

## SteMigrate

A migration and recovery workflow built around cpmove validation, staged
imports, auditability, verification, and incident response. The recovery
scenarios are documented in [`INCIDENT_LAB.md`](INCIDENT_LAB.md).

The portfolio standard is evidence over claims: each project should include
architecture, threat model, automated tests, a demo environment, measured
failure behavior, and a concise postmortem for one deliberately injected
incident.
