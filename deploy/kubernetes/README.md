# Kubernetes deployment

Create the secret out-of-band, then apply the manifest:

```sh
kubectl -n stepanel create secret generic stepanel-secrets \
  --from-literal=admin-password='change-me' \
  --from-literal=session-secret="$(openssl rand -hex 32)" \
  --from-literal=audit-key="$(openssl rand -hex 32)"
kubectl apply -f deploy/kubernetes/stepanel.yaml
```

Add `--from-literal=admin-totp-secret='BASE32_SECRET'` to require TOTP MFA.

The manifest creates two 10Gi persistent volume claims for control-plane data
and site files. Adjust their size and storage class for the environment. It
uses one replica with a `Recreate` rollout because restore jobs and persistent
state are process-local. The pod runs as UID/GID 10001 with a read-only root
filesystem and no service-account token. The manifest intentionally does not
enable trusted upstream TLS termination by default; add an HTTPS ingress and
set `STEPANEL_TLS_TERMINATED=1` only after that boundary is enforced. Label the
ingress namespace `stepanel.ingress=true`. The included NetworkPolicy denies
other ingress traffic.

This container is a control-plane packaging target, not a host-management
agent. Kubernetes pods do not have the host's Apache, PHP-FPM, systemd, local
accounts, or privileged StePanel helpers. Endpoints that provision host
services therefore remain unavailable in this topology. Use the systemd
installation for full host management; use Kubernetes only for explicitly
integrated remote services and persistent migration storage.
