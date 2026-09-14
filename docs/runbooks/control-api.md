# Control API alert runbook

These checks apply to the Prometheus rules in
`deploy/observability/prometheus-rules.yaml`. The alert expressions use only
the bounded, non-player-labelled metrics exported by `/metrics`.

## Control API unavailable

1. Check the deployment and readiness endpoint:

   ```sh
   kubectl -n platform-app get pods -l app.kubernetes.io/name=gameservice-control-api
   kubectl -n platform-app logs deploy/gameservice-control-api --since=15m
   curl -fsS http://gameservice-control-api:8080/health/ready
   ```

2. Check PostgreSQL reachability and the migration job before restarting API
   replicas. Do not delete the database volume or rerun down migrations.
3. If the failure is isolated to one replica, remove only that pod after
   collecting its logs. If all replicas fail, follow the PostgreSQL backup and
   restore runbook before any destructive recovery.

## High 5xx rate

1. Query the 5xx samples and correlate the time window with Control API logs.
2. Check PostgreSQL health, migration status, and outbox worker errors.
3. Roll back the application image or Helm revision only after preserving the
   failed revision and its audit/correlation IDs.

## No request traffic

Confirm the alert is not caused by a planned maintenance window, then check
the gateway route, Service endpoints, DNS, and the Prometheus scrape target.
This alert is not evidence that players are absent; it first indicates a
telemetry or routing problem.

## Production wiring

The Compose collector is a development transport and debug exporter. A
production deployment must load these rules into the approved Prometheus/
Alertmanager installation, route alerts to an on-call destination, and retain
the resulting alert history according to the launch RPO/RTO decision.
