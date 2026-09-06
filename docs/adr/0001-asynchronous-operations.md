# ADR-0001: Asynchronous long-running operations

- Status: Accepted
- Date: 2026-09-06

## Context

Restores, backups, certificates, cloud actions, and other host operations may
run longer than a reliable HTTP request and can consume significant resources.

## Decision

Perform admission and bounded preflight in the request, persist a job before
starting host work, return `202 Accepted`, and expose a polling endpoint. Jobs
use global and per-site concurrency limits and retain enough state to report
completion or interrupted failure after restart.

## Consequences

Operators receive durable status and bounded concurrency. Callers must poll
and handle a failed or pending job instead of assuming request completion.
