# Terraform deployment example

This example demonstrates the infrastructure boundary for StePlatform: a
cluster is owned by the environment, while Terraform owns the StePanel
namespace and workload manifest.

Before applying, pin `var.image` to a reviewed digest and create the
`stepanel-secrets` secret with `admin-password`, `session-secret`, independent
`audit-key`, mandatory `admin-totp-secret`, `account-key`, and `offsite-target` entries in the
target namespace. The offsite target must be a validated rclone destination.
The image includes the rclone client; provide its provider configuration and
credentials through the cluster's secret-management mechanism.
Then run:

```sh
terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

The example creates one replica with a `Recreate` rollout, 50Gi persistent claims
for control-plane data and site files, and an unprivileged pod with a read-only
root filesystem. The deployment assumes a trusted cluster ingress terminates
TLS before forwarding to the Service; do not expose its Service directly.
Label the ingress namespace `stepanel.ingress=true`. The Kubernetes provider is deliberately used instead of
provisioning a cloud account, keeping the example portable across managed or
on-premises clusters.

The control-plane claim includes durable sessions and, when shared-hosting
accounts are used, their bcrypt password hashes and TOTP seeds. Treat its
backup and restore path as encrypted sensitive control-plane state.
