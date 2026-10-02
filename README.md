# kube-image-warmer

A Kubernetes operator that discovers images from Deployments, StatefulSets,
DaemonSets, and CronJobs and creates warming Jobs on eligible nodes. It handles
an initial run, refreshes on a configured cron schedule, and warms newly eligible
nodes without waiting for the next scheduled run.

## Deployment and releases

The deployment is packaged as a Helm chart. GitHub Actions builds and publishes
the operator image and OCI chart. Your external deployment system installs or upgrades the release.
The chart installs the controller, ServiceAccount, RBAC, CRD, and optional policy.

- Operator image: `ghcr.io/denzil-briffa/kube-image-warmer:<version>`
- Helm chart: `oci://ghcr.io/denzil-briffa/charts/image-warmer`
- [Deployment configuration and automated release guide](docs/deployment.md)
- [Chart defaults](charts/image-warmer/values.yaml)
- [Example deployment values](deploy/values.yaml)

The cron expression is supplied through `operator.schedule` in Helm values.
Policy settings are supplied through `policy.spec`. The default policy is enabled
and includes the full supported workload and node scope; no namespace or node
filters are imposed. Node health checks and the concurrency limit still apply.

## Validate packaging locally

Run these in WSL from the repository root, with Go, Make, and Helm 3 available:

```bash
make fmt manifests generate
make lint-fix
make test
make helm-lint SCHEDULE='0 * * * *'
make helm-package VERSION=0.1.0 SCHEDULE='0 * * * *'
```

The validation schedule is only used while checking the chart. Packaged defaults
keep the schedule empty so each deployment must explicitly provide one. Packaging
copies generated CRDs and RBAC into the chart; edit source markers and regenerate
rather than editing those copies.