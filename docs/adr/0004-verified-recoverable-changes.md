# ADR-0004: Verified, recoverable changes

- Status: Accepted
- Date: 2026-09-06

## Context

Hosting changes can affect live files, databases, service units, and traffic
routing. Partial mutations are more dangerous than an explicit failed job.

## Decision

Inspect hostile archives before extraction, validate destination paths, persist
recovery journals before destructive site changes, verify backup artifacts and
optional signatures, apply desired state before reconciliation, and preserve a
rollback path whenever a change activates a new release or configuration.

## Consequences

Failures may leave an operation pending until reconciliation rather than
pretending it succeeded. Application rollback does not claim to reverse
database schema changes; operators must use the documented database recovery
workflow.
