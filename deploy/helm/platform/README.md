# Production Helm chart

This chart deploys the GameService control API, durable outbox worker,
Nakama leaderboard worker, migration hook, HA policy, HPA, and default-deny
network policy into `platform-app`. PostgreSQL and Nakama are external
dependencies; provide their URLs and credentials through the referenced Secret.

Every application image digest is required at render time. Example:

```sh
helm upgrade --install gameservice ./deploy/helm/platform \
  --namespace platform-app --create-namespace \
  --set images.controlApi.digest=sha256:... \
  --set images.outboxWorker.digest=sha256:... \
  --set images.leaderboardWorker.digest=sha256:... \
  --set images.migrations.digest=sha256:...
```

The chart is not a substitute for installing the pinned Agones release,
Nakama cluster, ingress, observability backend, or managed PostgreSQL HA.
