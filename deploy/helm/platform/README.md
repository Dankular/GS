# Production Helm chart

This chart deploys the GameService control API, durable outbox, leaderboard,
and Agones allocator/matchmaking workers, a migration hook, HA policies, HPA,
and default-deny network policy into `platform-app`. PostgreSQL and Nakama are external
dependencies; provide their URLs and credentials through the referenced Secret.

Every application image digest is required at render time. Example:

```sh
helm upgrade --install gameservice ./deploy/helm/platform \
  --namespace platform-app --create-namespace \
  --set images.controlApi.digest=sha256:... \
  --set images.outboxWorker.digest=sha256:... \
  --set images.leaderboardWorker.digest=sha256:... \
  --set images.matchmakingWorker.digest=sha256:... \
  --set images.migrations.digest=sha256:...
```

The chart is not a substitute for installing the pinned Agones release,
Nakama cluster, ingress, observability backend, or managed PostgreSQL HA. The
matchmaking worker requires the `gameservice-allocator-mtls` Secret containing
the configured CA and client certificate/key; it never receives Kubernetes API
credentials. All application containers run non-root with a RuntimeDefault
seccomp profile, dropped capabilities, read-only root filesystems, and a
temporary filesystem only where needed.
