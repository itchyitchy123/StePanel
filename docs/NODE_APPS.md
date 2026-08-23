# Node application deployment

Enable NVM during installation:

```sh
sudo STEPANEL_INSTALL_NODE=1 \
  STEPANEL_NODE_VERSIONS=20.18.0,22.14.0 ./install.sh
```

The panel lists versions installed in the StePanel service account's NVM
directory. Selecting a version writes a `.nvmrc` into the managed site root;
it does not execute an application or install packages on behalf of the user.

Deploying an app generates a managed Apache virtual host under
`/var/lib/ste-panel/proxy`. The backend must be an `http://` endpoint on
localhost or a private/link-local IP and must include a port. This prevents
the proxy endpoint from becoming an SSRF or open-proxy primitive.

The installer enables Apache proxy modules, includes managed snippets, and
installs `/usr/local/sbin/stepanel-apache-reload`, a root-owned helper that
validates Apache configuration before reloading it. HTTPS certificates and
application process supervision remain deployment responsibilities; use
systemd, PM2, or another supervised process manager for the Node app itself.

Managed proxies can be listed, backend-tested, and removed through the
authenticated proxy API. A failed Apache reload restores the previous proxy
configuration automatically.
