# Terraform deployment example

This example demonstrates the infrastructure boundary for StePlatform: a
cluster is owned by the environment, while Terraform owns the StePanel
namespace and workload manifest.

Before applying, pin `var.image` to a reviewed digest and create the
`stepanel-secrets` secret in the target namespace. Then run:

```sh
terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

The Kubernetes provider is deliberately used instead of provisioning a cloud
account. That keeps the example safe to review and portable across managed or
on-premises clusters.
