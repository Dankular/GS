# Production Helm chart

This chart deploys the GameService control API, durable outbox, leaderboard,
Agones allocator/matchmaking, and reconciliation workers, a migration hook, HA policies, HPA,
and default-deny network policy into `platform-app`. PostgreSQL and Nakama are external
dependencies; provide their URLs and credentials through the referenced Secret.

The optional `postgresCluster` values enable a CloudNativePG `Cluster` in the
application namespace. It is disabled by default and requires an immutable
PostgreSQL image digest. Enabling it does not synthesize the application's
`database-url` Secret: provide that Secret through the configured secret
manager after validating the generated CloudNativePG service and credentials.
Use at least three instances and failure-domain-aware storage in a real
multi-node cluster; the single-node Kind/VPS environment is only suitable for
rendering and operator smoke tests, not an HA claim.

Every application image digest is required at render time. Example:

```sh
helm upgrade --install gameservice ./deploy/helm/platform \
  --namespace platform-app --create-namespace \
  --set images.controlApi.digest=sha256:... \
  --set images.outboxWorker.digest=sha256:... \
  --set images.leaderboardWorker.digest=sha256:... \
  --set images.matchmakingWorker.digest=sha256:... \
  --set images.reconciliationWorker.digest=sha256:... \
  --set images.migrations.digest=sha256:...
```

The chart is not a substitute for installing the pinned Agones release,
Nakama cluster, ingress, observability backend, or managed PostgreSQL HA. The
matchmaking worker requires the `gameservice-allocator-mtls` Secret containing
the configured CA and client certificate/key; it never receives Kubernetes API
credentials. All application containers run non-root with a RuntimeDefault
seccomp profile, dropped capabilities, read-only root filesystems, and a
temporary filesystem only where needed.

The production example enables the Kyverno image-signature admission policy.
Install a compatible Kyverno release before applying those values, and keep
the keyless issuer, subject, and Rekor settings aligned with the release
workflow. Without Kyverno installed, the production admission gate cannot be
verified.

Set `ingress.enabled=true` and supply TLS through the release system to expose
the Control API through an approved ingress controller. The checked-in
`values.production.example.yaml` is a topology example only; it deliberately
contains no credentials or image digests.

The referenced Secret must also contain `nakama-runtime-http-key` when a
definition enables tournament delivery. The leaderboard worker uses that key
only for the narrow server-to-server `gameservice.tournament_record` runtime
RPC; it never exposes the key to clients.

For secret-manager deployments, install External Secrets Operator and provide
an approved `ClusterSecretStore`, then enable `externalSecrets` in the release
values. The chart creates `ExternalSecret` resources in both the application
and gameserver namespaces from the configured remote key; no provider
credentials are stored in this repository. The default remains disabled so a
release cannot silently depend on an uninstalled operator.
