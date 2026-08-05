package globals

import (
	"context"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// clearOTELEnv blanks the standard OpenTelemetry env vars so a disabled config
// stays disabled regardless of the host environment.
func clearOTELEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		t.Setenv(k, "")
	}
}

func TestInitTracingDisabled(t *testing.T) {
	clearOTELEnv(t)
	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil config", cfg: nil},
		{name: "otlp disabled in config", cfg: &config.Config{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracerShutdown = nil
			Cfg = tt.cfg
			if err := InitTracing(context.Background()); err != nil {
				t.Fatalf("InitTracing() error = %v, want nil", err)
			}
			if tracerShutdown == nil {
				t.Fatal("tracerShutdown must be set even when tracing is disabled")
			}
		})
	}
}

func TestInitTracingEnabledAndShutdown(t *testing.T) {
	clearOTELEnv(t)
	origCfg := Cfg
	t.Cleanup(func() { Cfg = origCfg })

	Cfg = &config.Config{}
	Cfg.Observability.OTLP = config.OTLPConfig{
		Enabled:  true,
		Endpoint: "127.0.0.1:4318",
		Insecure: true,
		Headers:  map[string]string{"test": "1"},
	}
	tracerShutdown = nil

	if err := InitTracing(context.Background()); err != nil {
		t.Fatalf("InitTracing(enabled) error = %v, want nil", err)
	}
	if tracerShutdown == nil {
		t.Fatal("tracerShutdown must be set after enabled InitTracing")
	}
	ShutdownTracing(context.Background())
	if tracerShutdown != nil {
		t.Error("tracerShutdown must be cleared by ShutdownTracing")
	}
}

func TestShutdownTracingNoopWhenUnset(t *testing.T) {
	tracerShutdown = nil
	ShutdownTracing(context.Background())
}
