# ADR-0005: Incremental Go package boundaries

- Status: Accepted
- Date: 2026-09-06

## Context

StePanel has grown into a substantial single-host control plane, but its
feature files still share one root Go package. A large mechanical conversion
would increase regression risk because handlers, persistence stores, platform
helpers, and test fixtures currently share internal types.

## Decision

Keep the current buildable package stable while extracting tested seams in
small steps. New boundaries will be introduced in this order:

1. authentication and session policy;
2. durable jobs and operation serialization;
3. deployment and platform adapters;
4. backups and migration workflows;
5. resource enforcement and reconciliation.

The first completed extractions are `internal/session`, `internal/deployment`,
`internal/operations`, `internal/usage`, and the platform-neutral
`internal/state` atomic writer. The remaining authentication, job, platform,
backup, and resource seams are intentionally still coupled to the HTTP assembly
layer.

Each extraction must retain the existing integration tests, add package-level
contract tests, and leave HTTP routing as an assembly concern. No package is
created solely to move files; it must own a stable interface and reduce
coupling.

## Consequences

The repository remains easy to build during the transition, while the
architecture has an explicit path away from an oversized root package. The
short-term cost is that some root-package coupling remains until the relevant
interfaces stabilize.
