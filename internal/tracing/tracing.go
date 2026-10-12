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

// defaultOTLPEndpoint is the OTLP/HTTP collector the SDK uses when neither the
// config endpoint nor OTEL_EXPORTER_OTLP_ENDPOINT is set.
const defaultOTLPEndpoint = "localhost:4318"

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
	if !cfg.Enabled && !EnvEnabled() {
		return noop, nil
	}

	exporter, err := otlptracehttp.New(ctx, exporterOptions(cfg)...)
	if err != nil {
		return noop, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(buildResource(cfg.Version)),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// exporterOptions translates Config into OTLP/HTTP exporter options.
func exporterOptions(cfg Config) []otlptracehttp.Option {
	var opts []otlptracehttp.Option
	if cfg.Endpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}
	return opts
}

// buildResource builds the OTel resource describing this service.
func buildResource(version string) *resource.Resource {
	if version == "" {
		version = "dev"
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("ludus"),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		return resource.Default()
	}
	return res
}

// ResolvedOTLPEndpoint returns the effective OTLP collector endpoint: the
// configured one when set, then the standard OTEL_* environment variables in
// SDK precedence (signal-specific, then general), and the SDK default
// otherwise.
func ResolvedOTLPEndpoint(endpoint string) string {
	if endpoint != "" {
		return endpoint
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); v != "" {
		return v
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		return v
	}
	return defaultOTLPEndpoint
}

// Tracer returns the ludus tracer from the global provider. When tracing is not
// initialized, the global provider is a no-op and spans are cheap nil ops.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// OtlpState describes whether OTLP export is active and where it goes.
type OtlpState struct {
	Active   bool
	Source   string // "config" or the OTEL_* variable name when env-driven; "" when inactive
	Endpoint string // resolved endpoint; "" when inactive
}

// OtelEnvVars are the standard OpenTelemetry variables that opt into OTLP
// export, in the order they are reported.
var OtelEnvVars = []string{
	"OTEL_TRACES_EXPORTER",
	"OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
}

// EnvSource reports the name of the standard OTEL_* variable currently opting
// the environment into export, or "" when none is set.
func EnvSource() string {
	for _, name := range OtelEnvVars {
		if v := os.Getenv(name); v != "" && v != "none" {
			return name
		}
	}
	return ""
}

// EnvEnabled reports whether the standard OpenTelemetry environment opts the
// user into export, so env-only configuration works without setting Enabled.
func EnvEnabled() bool {
	return EnvSource() != ""
}

// Diagnose reports the effective OTLP export state: whether export is active,
// which source (config or environment variable) enabled it, and the resolved
// collector endpoint.
func Diagnose(cfg Config) OtlpState {
	if !cfg.Enabled {
		if src := EnvSource(); src != "" {
			return OtlpState{Active: true, Source: src, Endpoint: ResolvedOTLPEndpoint(cfg.Endpoint)}
		}
		return OtlpState{}
	}
	return OtlpState{Active: true, Source: "config", Endpoint: ResolvedOTLPEndpoint(cfg.Endpoint)}
}

// UseRecorder installs an in-memory tracer provider backed by the given recorder
// and returns a restore func. Intended for tests.
func UseRecorder(rec sdktrace.SpanProcessor) func() {
	prev := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	return func() { otel.SetTracerProvider(prev) }
}
