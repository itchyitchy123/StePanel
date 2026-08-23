# StePanel Helm chart

The chart is a packaging example for StePlatform. Render it before applying:

```sh
helm template stepanel deploy/helm/stepanel
helm upgrade --install stepanel deploy/helm/stepanel \
  --set image.tag=v0.1.0
```

Create `stepanel-secrets` separately. Production installations should pin an
image digest, configure a persistent import volume, and add an ingress with
TLS and a network policy.
