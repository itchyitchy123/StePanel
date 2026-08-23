# StePanel Helm chart

The chart is a packaging example for StePlatform. Render it before applying:

```sh
helm template stepanel deploy/helm/stepanel
helm upgrade --install stepanel deploy/helm/stepanel \
  --set image.tag=v0.1.0
```

Create `stepanel-secrets` separately. Production installations should pin an
image digest, select an appropriate persistent storage class, and add an
ingress with TLS and a network policy. The chart defaults to one replica
because restore jobs and managed site state are local to the control plane.
