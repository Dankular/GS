# Observability

`otel-collector` is included in the Docker Compose core stack. It accepts OTLP
over gRPC on `4317` and HTTP on `4318`, and exposes collector-promoted
Prometheus metrics on `9464`. The development collector uses the bounded debug
exporter; production requires an approved durable backend and alert rules.
