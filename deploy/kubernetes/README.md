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
filesystem and no service-account token. Add an ingress, TLS, database
connection, and environment-specific network policy before production use.
