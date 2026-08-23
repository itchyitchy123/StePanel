# Kubernetes deployment

Create the secret out-of-band, then apply the manifest:

```sh
kubectl -n stepanel create secret generic stepanel-secrets \
  --from-literal=admin-password='change-me' \
  --from-literal=session-secret="$(openssl rand -hex 32)"
kubectl apply -f deploy/kubernetes/stepanel.yaml
```

The manifest creates two 10Gi persistent volume claims for control-plane data
and site files. Adjust their size and storage class for the environment. It
uses one replica because restore jobs are currently process-local. Add an
ingress, TLS, database connection, and network policy before production use.
