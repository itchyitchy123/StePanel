# Terraform deployment example

This example demonstrates the infrastructure boundary for StePlatform: a
cluster is owned by the environment, while Terraform owns the StePanel
namespace and workload manifest.

Before applying, pin `var.image` to a reviewed digest and create the
`stepanel-secrets` secret with `admin-password`, `session-secret`, and
independent `audit-key` entries in the target namespace. The optional
`admin-totp-secret` entry enables MFA. Then run:

```sh
terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

The example creates one replica with a `Recreate` rollout, persistent claims
for control-plane data and site files, and an unprivileged pod with a read-only
root filesystem. The pod explicitly expects TLS to be terminated by a trusted
cluster ingress; do not expose its Service directly. The Kubernetes provider is deliberately used instead of
provisioning a cloud account, keeping the example portable across managed or
on-premises clusters.
