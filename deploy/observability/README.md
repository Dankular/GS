# Observability

`otel-collector` is included in the Docker Compose core stack. It accepts OTLP
over gRPC on `4317` and HTTP on `4318`, and exposes collector-promoted
Prometheus metrics on `9464`. The development collector uses the bounded debug
exporter; production requires an approved durable backend and alert rules.
The repository's baseline Prometheus rules are in
`deploy/observability/prometheus-rules.yaml`; each alert links to a runbook in
`docs/runbooks/`. They must be loaded into the approved production
Prometheus/Alertmanager installation before launch. The development collector
does not provide durable metrics, alert delivery, or an SLO dashboard.
