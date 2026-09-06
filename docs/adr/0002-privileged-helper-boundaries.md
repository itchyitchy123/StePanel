# ADR-0002: Narrow privileged helper boundaries

- Status: Accepted
- Date: 2026-09-06

## Context

Site files, service units, databases, and webserver configuration require
privileged access, but a root-running HTTP process would make a panel breach a
host-wide command-execution event.

## Decision

Run the control plane as the restricted `stepanel` account. Cross privilege
boundaries only through root-owned, allowlisted helpers that receive typed
arguments, validate independently, bound output and duration, and perform
their own final filesystem/service checks.

## Consequences

The implementation has more explicit integration surfaces to maintain, but a
web/API validation mistake does not automatically grant arbitrary root shell
access. Helper changes require separate review and host-level tests.
