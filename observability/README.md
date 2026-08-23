# StePanel observability

This directory is the seed of **SteObserve**, a focused observability product
for hosting control planes and PHP workloads. It is intentionally deployable
with standard Prometheus and Grafana rather than requiring a proprietary
agent.

## Quick start

1. Run StePanel with `/metrics` reachable by your Prometheus server.
2. Add [`prometheus.yml`](prometheus.yml) to the Prometheus configuration or
   merge its scrape job into an existing configuration.
3. Import [`grafana-dashboard.json`](grafana-dashboard.json) into Grafana.
4. Alert on failed restores and sustained active jobs using the examples in
   [`../docs/SLO.md`](../docs/SLO.md).

The metrics endpoint is deliberately low-cardinality: account names, backup
paths, and error text are never emitted as labels.
