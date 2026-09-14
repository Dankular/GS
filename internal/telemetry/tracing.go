package telemetry

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Setup installs a process tracer when endpoint is configured. An empty
// endpoint intentionally leaves the SDK as a no-op for local unit tests.
func Setup(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if strings.TrimSpace(serviceName) == "" {
		return nil, errors.New("telemetry service name is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		return nil, err
	}
	provider := trace.NewTracerProvider(trace.WithBatcher(exporter), trace.WithResource(res))
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}
