# Git site deployments

StePanel can deploy a pre-built public site tree from an HTTPS Git repository.
The operation is intended for static sites and repositories whose committed
contents are already deployable. StePanel does not execute repository scripts,
package managers, hooks, or build commands inside the control-plane process.

## Prerequisites

- Create the site and its isolated site root before deployment.
- Install the `git` client on the host.
- Add every permitted repository hostname to
  `STEPANEL_GIT_ALLOWED_HOSTS`. The default is
  `github.com,gitlab.com,bitbucket.org`.
- Use a repository URL without embedded credentials, query parameters, or a
  nonstandard port. Private-repository credentials are not accepted.

## Deploy

Send an authenticated JSON request with the session CSRF token:

```sh
curl -X POST https://panel.example.test/api/sites/git-deploy \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <session token>' \
  --data '{"site":"example","repository":"https://github.com/acme/site.git","ref":"main"}'
```

StePanel performs a shallow, single-ref checkout with credential prompting and
Git credential helpers disabled. It verifies the commit identifier, rejects
symlinks and special files, applies the configured entry limit, removes `.git`,
and atomically replaces the site's `public` directory. The prior directory is
retained under the site root as `.stepanel-previous-<id>`.

Only the final filesystem switch is serialized. Repository transfer happens
before that lock so unrelated checkouts do not block one another. The request
has a ten-minute timeout and returns the exact deployed commit on success.

## Roll back

Rollback activates the newest preserved release and retains the release it
replaces:

```sh
curl -X POST https://panel.example.test/api/sites/git-rollback \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <session token>' \
  --data '{"site":"example","confirm":"ROLLBACK example"}'
```

The preserved release is safety-checked again before activation. Deployment
and rollback restore site ownership/isolation through the site helper and write
tamper-evident audit events.

## Operational boundaries

- Git deployment changes site files only. It does not migrate databases,
  change runtime configuration, restart Node, or issue certificates.
- The repository host allowlist is exact; subdomains are not implicitly trusted.
- Use a dedicated sandboxed CI/build runner for Composer, npm, framework builds,
  tests, artifact signing, and secret injection. Deploy only reviewed output.
- Preserved releases consume site storage and currently require operator-managed
  retention. Monitor disk usage and retain backups independently.
- Database changes require a separate migration and rollback plan.
