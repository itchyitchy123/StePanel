# ADR-0003: Caddy as the default webserver

- Status: Accepted
- Date: 2026-09-06

## Context

New installations need a secure, low-maintenance webserver while cPanel
migrations still require compatibility with Apache and OpenLiteSpeed.

## Decision

Use Caddy by default for automatic HTTPS and a compact configuration model.
Keep Apache and OpenLiteSpeed as explicit alternatives, with webserver-specific
helpers, complete configuration validation, and clear capability reporting.

## Consequences

Modern sites get a simpler TLS lifecycle. Apache-only integrations such as
native ModSecurity and manual Certbot issuance must remain visibly scoped, and
`.htaccess` conversion stays fail-closed rather than guessing unsupported
behavior.
