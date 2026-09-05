# Node application deployment

Enable NVM during installation:

```sh
sudo STEPANEL_INSTALL_NODE=1 \
  STEPANEL_ADMIN_PASSWORD='use-a-password-manager' \
  STEPANEL_PANEL_HOSTNAME=panel.example.com \
  STEPANEL_NODE_VERSIONS=20.18.0,22.14.0 ./install.sh
```

The panel lists versions installed in the StePanel service account's NVM
directory. Selecting a version writes a `.nvmrc` into the managed site root;
it does not execute an application or install packages on behalf of the user.
Managed Node units run as the deterministic Unix identity created for that
site, with a unique primary group and write access limited to that site's
public root.

Deploying an app asks the root-owned `stepanel-proxyctl` helper to generate a
managed Apache virtual host under `/etc/apache2/stepanel-proxy` or
`/etc/httpd/conf.d/stepanel-proxy`. The service account cannot write those
directories. The backend must be an `http://` endpoint on
localhost or a private/link-local IP and must include a port. This prevents
the proxy endpoint from becoming an SSRF or open-proxy primitive.

The installer enables Apache proxy modules, includes the root-owned managed
snippets, and installs `/usr/local/sbin/stepanel-proxyctl`. The helper validates
all arguments, renders a fixed virtual-host template, checks the complete
Apache configuration, and rolls back a failed reload. Optional HTTPS issuance is
described in `docs/CERTIFICATES.md`. Application process supervision is
provided by the managed systemd unit; use the rollback endpoint when a release
needs to be reverted.

Managed proxies can be listed, backend-tested, and removed through the
authenticated proxy API. A failed Apache reload restores the previous proxy
configuration automatically.

Git site deployment is independent of Node process deployment. It can
atomically replace and roll back committed site files, but it does not run
`npm install`, execute build scripts, restart the systemd unit, or alter the
proxy. See [`GIT_DEPLOYMENTS.md`](GIT_DEPLOYMENTS.md) and coordinate file,
process, proxy, and database changes as separate audited operations.
