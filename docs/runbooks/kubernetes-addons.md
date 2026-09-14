# Kubernetes add-ons

The configured VPS Kind validation cluster currently has these pinned add-ons:

| Add-on | Release | Purpose |
| --- | --- | --- |
| Agones | Helm chart `1.60.0` | Dedicated-server lifecycle and allocation |
| External Secrets Operator | Helm chart `2.10.0` / app `v2.10.0` | Synchronize provider-managed secrets into Kubernetes |

The VPS cluster is a single-node validation environment. It proves chart/API
compatibility, not failure-domain high availability.

## External Secrets Operator

Install or reconcile the operator with:

```sh
helm repo add external-secrets https://charts.external-secrets.io
helm repo update
helm upgrade --install external-secrets external-secrets/external-secrets \
  --namespace external-secrets --create-namespace \
  --version 2.10.0 --wait --timeout 180s
kubectl get deployment,pods -n external-secrets
kubectl get crd externalsecrets.external-secrets.io \
  secretstores.external-secrets.io clustersecretstores.external-secrets.io
```

The GameService chart's `externalSecrets.enabled` option remains disabled by
default. Production values enable it and reference an operator-managed
`ClusterSecretStore`; the provider, workload identity, remote secret key, and
rotation policy must be supplied by the deployment owner. Do not create a
placeholder provider or put cloud credentials in this repository.

## GameService chart ordering

Install the operator and Agones before rendering a production-shaped
GameService release. The GameService release then creates `ExternalSecret`
resources in `platform-app` and `platform-gameservers-<region>`, followed by
the migration hook and application workloads. A release with
`externalSecrets.enabled=true` must not be considered ready until the synced
target Secrets have `Ready=True` conditions.
