# Observability

`otel-collector` is included in the Docker Compose core stack. It accepts OTLP
over gRPC on `4317` and HTTP on `4318`, and exposes collector-promoted
Prometheus metrics on `9464`. The development collector uses the bounded debug
exporter; production requires an approved durable backend and alert rules.
The repository's baseline Prometheus rules are in
`deploy/observability/prometheus-rules.yaml`; each alert links to a runbook in
`docs/runbooks/`. They must be loaded into the approved production
Prometheus/Alertmanager installation before launch. The development collector
does not provide durable metrics or alert delivery. The importable baseline
dashboard is `deploy/observability/gameservice-dashboard.json`; it is an
artifact for the approved production Grafana installation, not a claim that
Grafana is deployed by the Compose stack.
