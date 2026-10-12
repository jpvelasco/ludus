package globals

import (
	"context"
	"fmt"
	"os"

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

// WarnIfEnvOTLP prints a one-time stderr notice when the standard OTEL_*
// environment enables trace export while no Ludus config does, so a shell
// with org-wide OTEL variables does not export spans silently. No-op when
// the config explicitly enables OTLP (the user asked for it) or when neither
// source is active.
func WarnIfEnvOTLP() {
	if Cfg == nil || Cfg.Observability.OTLP.Enabled {
		return
	}
	// Endpoint carries the config value so the reported endpoint matches what
	// InitTracing resolves; Enabled stays false (we only warn in the env path).
	state := tracing.Diagnose(tracing.Config{Endpoint: Cfg.Observability.OTLP.Endpoint})
	if state.Active {
		fmt.Fprintf(os.Stderr, "Note: OTLP trace export is enabled by %s (no Ludus config change); spans go to %s\n", state.Source, state.Endpoint)
	}
}
