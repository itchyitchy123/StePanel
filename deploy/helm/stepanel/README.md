# StePanel Helm chart

The chart is a packaging example for StePlatform. Render it before applying:

```sh
helm template stepanel deploy/helm/stepanel
helm upgrade --install stepanel deploy/helm/stepanel \
  --set image.tag=v0.3.0
```

Create `stepanel-secrets` separately with `admin-password`, `session-secret`,
and an independent `audit-key`; `admin-totp-secret` is optional. Production installations should pin an
image digest, select an appropriate persistent storage class, enable and
configure the ingress with TLS, and add a network policy appropriate to the
cluster ingress controller. The chart enforces one replica because restore
jobs and managed site state are local to the control plane.

The chart packages the control plane only. It does not grant access to a
Kubernetes node's Apache, PHP-FPM, systemd, accounts, or StePanel privileged
helpers, so host-provisioning endpoints are unavailable unless a separate,
explicit integration is supplied. The systemd deployment remains the full
host-management topology.
