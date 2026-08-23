# Kubernetes deployment

Create the secret out-of-band, then apply the manifest:

```sh
kubectl -n stepanel create secret generic stepanel-secrets \
  --from-literal=admin-password='change-me' \
  --from-literal=session-secret="$(openssl rand -hex 32)"
kubectl apply -f deploy/kubernetes/stepanel.yaml
```

The image, database connection, ingress, persistent storage, and network
policy should be supplied by the environment. The sample intentionally does
not embed credentials or a cloud-specific load balancer.
