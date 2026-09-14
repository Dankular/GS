# Kubernetes add-ons

The configured VPS Kind validation cluster currently has these pinned add-ons:

| Add-on | Release | Purpose |
| --- | --- | --- |
| Agones | Helm chart `1.60.0` | Dedicated-server lifecycle and allocation |
| External Secrets Operator | Helm chart `2.10.0` / app `v2.10.0` | Synchronize provider-managed secrets into Kubernetes |
| ingress-nginx | Helm chart `4.15.1` / controller `v1.15.1` | Ingress routing and ModSecurity/OWASP CRS validation |

The VPS cluster is a single-node validation environment. It proves chart/API
compatibility, not failure-domain high availability.

## ingress-nginx and WAF

The validation controller is installed with a NodePort and global ModSecurity
plus OWASP CRS enabled:

```sh
helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace --version 4.15.1 \
  --set controller.service.type=NodePort \
  --set controller.config.enable-modsecurity=true \
  --set controller.config.enable-owasp-modsecurity-crs=true --wait
kubectl get deployment,pods,svc -n ingress-nginx
kubectl get configmap ingress-nginx-controller -n ingress-nginx -o yaml \
  | grep -E 'enable-modsecurity|enable-owasp-modsecurity-crs'
```

The production example enables the corresponding Ingress annotations and TLS
redirect. A real production deployment still needs an approved TLS certificate,
DNS, external load balancer, and WAF tuning/false-positive review.

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

## CloudNativePG (optional PostgreSQL operator)

The VPS has the CloudNativePG Helm repository configured. The pinned operator
baseline for the functional Kind environment is chart `0.29.0` / operator
`1.30.0`:

```sh
helm upgrade --install cnpg cnpg/cloudnative-pg \
  --version 0.29.0 --namespace cnpg-system --create-namespace
kubectl -n cnpg-system rollout status deployment/cnpg-cloudnative-pg
```

The platform chart's `postgresCluster.enabled` option emits a CloudNativePG
`postgresql.cnpg.io/v1` `Cluster`, with three instances by default and an
immutable PostgreSQL image digest requirement. It is intentionally opt-in:
managed PostgreSQL remains the recommended production boundary, and the
application's `database-url` Secret must still be supplied by the approved
secret manager. Validate storage classes, topology spread, fencing, and
off-site WAL/archive backups before treating a deployment as HA or PITR-ready.
