package globals

import (
	"context"

	"github.com/jpvelasco/ludus/internal/tracing"
	"github.com/jpvelasco/ludus/internal/version"
)

// tracerShutdown flushes/stops the OTLP tracer provider on exit. No-op if
// tracing was never enabled.
var tracerShutdown tracing.ShutdownFunc

// InitTracing initializes OTLP trace export from the loaded config. Safe to call
// once after Cfg is set. A nil/disabled config installs a no-op provider.
func InitTracing(ctx context.Context) error {
	var cfg tracing.Config
	if Cfg != nil {
		o := Cfg.Observability.OTLP
		cfg = tracing.Config{
			Enabled:  o.Enabled,
			Endpoint: o.Endpoint,
			Insecure: o.Insecure,
			Headers:  o.Headers,
			Version:  version.Version,
		}
	}
	shutdown, err := tracing.Init(ctx, cfg)
	tracerShutdown = shutdown
	return err
}

// ShutdownTracing flushes and stops the tracer provider. Always safe to call.
func ShutdownTracing(ctx context.Context) {
	if tracerShutdown != nil {
		_ = tracerShutdown(ctx)
		tracerShutdown = nil
	}
}
