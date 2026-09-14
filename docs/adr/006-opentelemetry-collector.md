# ADR-006: OpenTelemetry Collector boundary

## Decision

The Compose development profile runs a pinned OpenTelemetry Collector Contrib
container as the telemetry ingress. Services send OTLP signals to the
collector; the collector exposes Prometheus metrics and emits bounded debug
output for local verification. Services do not depend on a vendor-specific
backend.

The collector is a transport and processing boundary, not a source of domain
truth. Correlation IDs remain explicit fields on API, command, match, and
outbox records. Player, match, and request identifiers must not become metric
labels.

## Consequences

The development stack has a stable OTLP endpoint and Prometheus scrape target.
Production must replace the debug exporter with an approved durable telemetry
backend and add retention, access control, and alerting before launch.
