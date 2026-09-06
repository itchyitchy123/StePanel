# ADR-0006: Keyed serialization of site mutations

- Status: Accepted
- Date: 2026-09-06

## Context

Several operations mutate the same site-owned state: routes, PHP runtime,
applications, workers, scheduled tasks, environments, resources, and restore
transactions. Serializing only individual endpoints permits concurrent requests
to race over shared files, systemd units, or service reloads.

## Decision

Use a keyed operation registry and acquire the site key before a mutation. A
multi-site operation acquires all affected keys in sorted order, preventing
deadlocks while allowing unrelated sites to continue concurrently.

Every host operation remains bounded by a context timeout. Persisted pending
state and startup reconciliation handle the case where the host mutation
finishes but the final state write does not.

## Alternatives considered

- A single global mutex: simpler, but unnecessarily serializes independent
  sites and reduces useful concurrency.
- Endpoint-specific locks: easy to introduce, but incomplete as new features
  are added and unable to express cross-site operations.
- Optimistic versioning alone: useful for durable state, but insufficient to
  prevent simultaneous systemd/webserver mutations against the same site.

## Consequences

Same-site operations are deliberately serialized. A long-running operation can
delay another operation for that site, so callers must use bounded contexts and
surface pending/reconciliation status. The registry is an in-process safety
boundary; a future multi-process control plane must move the lock/lease into
durable state.
