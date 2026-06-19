// Package tracing provides optional OpenTelemetry (OTLP) trace export for ludus
// builds. It is a no-op unless explicitly enabled via config or the standard
// OTEL_* environment variables, so there is zero overhead in the common case.
package tracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/jpvelasco/ludus"

// Config controls OTLP trace export.
type Config struct {
	Enabled  bool
	Endpoint string
	Insecure bool
	Headers  map[string]string
	Version  string
}

// ShutdownFunc flushes and stops the tracer provider. Always safe to call.
type ShutdownFunc func(context.Context) error

// Init installs a global OTLP tracer provider when enabled. When disabled, it
// installs nothing and returns a no-op shutdown. Standard OTEL_* env vars are
// honored by the exporter; passing Endpoint overrides the env endpoint.
// Export failures are non-fatal (the SDK retries/drops in the background).
func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	noop := func(context.Context) error { return nil }
	if !cfg.Enabled && !otelEnvEnabled() {
		return noop, nil
	}

	opts := []otlptracehttp.Option{}
	if cfg.Endpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return noop, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("ludus"),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// otelEnvEnabled reports whether the standard OpenTelemetry environment opts the
// user into export, so env-only configuration works without setting cfg.Enabled.
func otelEnvEnabled() bool {
	if v := os.Getenv("OTEL_TRACES_EXPORTER"); v != "" && v != "none" {
		return true
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}

// Tracer returns the ludus tracer from the global provider. When tracing is not
// initialized, the global provider is a no-op and spans are cheap nil ops.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// UseRecorder installs an in-memory tracer provider backed by the given recorder
// and returns a restore func. Intended for tests.
func UseRecorder(rec sdktrace.SpanProcessor) func() {
	prev := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	return func() { otel.SetTracerProvider(prev) }
}
