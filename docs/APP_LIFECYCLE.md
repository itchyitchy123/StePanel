# Node application lifecycle

The application deployment endpoint creates a per-site manifest and asks the
root-owned `stepanel-appctl` helper to create a restricted systemd unit. The
unit runs `npm start` with the selected NVM version, restarts after failure, and
only writes inside the managed site root.

Required deployment fields are:

- site account
- installed Node version
- application port between 1024 and 65535
- optional domain for the proxy configuration

The application must already exist in the site's `public` directory and must
provide a working `npm start` script. Upload/build pipelines, environment
secret storage, HTTPS certificates, and atomic release rollback remain separate
deployment steps.
