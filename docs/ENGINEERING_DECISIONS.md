# Engineering decisions

StePanel is intentionally designed as a small control plane around existing
Linux services. These are the decisions worth discussing in a design review,
technical interview, or project walkthrough.

## Why are restores asynchronous?

A cPanel restore can upload and inspect a large archive, write many site files,
provision databases, and reload a webserver. Keeping that work inside the HTTP
request would create unreliable timeouts and make concurrent restores difficult
to control.

The API performs admission and preflight validation, persists a job record, and
returns `202 Accepted` with a polling URL. A bounded worker then performs the
restore with global and per-site limits. Job state is durable, so a restart can
preserve completed results and mark interrupted work failed. The same model is
used for backups, certificates, cloud actions, SSH actions, and other
long-running operations.

## How is archive path traversal prevented?

Archives are treated as hostile input. The upload and entry count are bounded;
gzip/tar streams are parsed before restore; absolute paths, `..` components,
symlinks, and special files are rejected; and the archive is staged in a
private, timestamped directory. Destination paths are checked to remain inside
the account-specific root before files are copied.

Inspection and restore are separate operations. Inspection returns metadata and
findings without activating content, while restore requires an explicit
operator confirmation and an account-scoped plan. SQL destinations are newly
created account-prefixed databases, and existing destinations are refused.

## Why is Caddy the default webserver?

Caddy gives a small self-hosted installation a sensible secure default: HTTPS
automation, a compact configuration model, and a straightforward reverse
proxy path for PHP-FPM and local applications. Apache and OpenLiteSpeed remain
supported for compatibility with existing hosting environments and migrations.

The choice is an operational default, not an abstraction that hides the
webserver. StePanel renders managed configuration through a selected helper,
validates the complete configuration, and reloads only after validation. A
failed validation or reload restores the prior managed configuration.

## How are privileged operations handled?

The HTTP service runs as an unprivileged `stepanel` account. It does not receive
general root shell access. Operations that need elevated permissions cross
narrow, root-owned helpers such as the site, vhost, proxy, application, and
database helpers.

The control plane passes typed, validated arguments as separate process
arguments using context-bound commands with bounded output. Helpers validate
again, use fixed allowlists and templates, and own the final filesystem and
service checks. This creates a second enforcement boundary if an API check is
ever bypassed or a future endpoint is implemented incorrectly.

## What happens if an offsite backup fails?

The local archive is created and verified before publication. When an offsite
target is configured, the job then uploads it through a constrained `rclone`
invocation using `--immutable`. If that upload fails, the job is reported as
failed and the failure is audited; scheduled backups also record the error and
increment their consecutive-failure count. A local artifact may still exist,
but it is not treated as a successful protected backup until the offsite step
completes.

This separates artifact integrity from durability. The archive digest proves
what was written locally; offsite success is a separate operational guarantee.
Operators should alert on failed jobs, retain local artifacts according to
policy, and verify the remote copy independently.

## What is deliberately out of scope?

StePanel is an operator-focused single-administrator control plane, not yet a
multi-tenant hosting platform. Tenant role separation, customer-scoped
privileged restores, and OIDC remain explicit roadmap items. Calling out those
limits is part of the security model: the current design assumes a trusted
operator and host-level isolation between the panel and site workloads.

See the [architecture and lifecycle diagram](ARCHITECTURE.md),
[threat model](THREAT_MODEL.md), [security policy](../SECURITY.md), and
[five-minute demo](DEMO.md) for implementation and operational detail.
