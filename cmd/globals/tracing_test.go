package globals

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/tracing"
)

// clearOTELEnv blanks the standard OpenTelemetry env vars so a disabled config
// stays disabled regardless of the host environment.
func clearOTELEnv(t *testing.T) {
	t.Helper()
	for _, k := range tracing.OtelEnvVars {
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

// captureStderrText swaps os.Stderr with a pipe for fn and returns what it wrote.
func captureStderrText(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = previous
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWarnIfEnvOTLP(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		env  map[string]string
		want string
	}{
		{
			name: "env-only emits notice with env endpoint",
			cfg:  &config.Config{},
			env:  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://org-collector:4318"},
			want: "OTEL_EXPORTER_OTLP_ENDPOINT",
		},
		{
			name: "config endpoint overrides env endpoint in notice",
			cfg:  &config.Config{Observability: config.ObservabilityConfig{OTLP: config.OTLPConfig{Endpoint: "config:4318"}}},
			env:  map[string]string{"OTEL_TRACES_EXPORTER": "otlp"},
			want: "config:4318",
		},
		{
			name: "config enabled suppresses the notice",
			cfg:  &config.Config{Observability: config.ObservabilityConfig{OTLP: config.OTLPConfig{Enabled: true}}},
			env:  map[string]string{"OTEL_TRACES_EXPORTER": "otlp"},
			want: "",
		},
		{
			name: "nothing enabled emits nothing",
			cfg:  &config.Config{},
			env:  nil,
			want: "",
		},
		{
			name: "nil config emits nothing",
			cfg:  nil,
			env:  map[string]string{"OTEL_TRACES_EXPORTER": "otlp"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearOTELEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			origCfg := Cfg
			t.Cleanup(func() { Cfg = origCfg })
			Cfg = tt.cfg

			out := captureStderrText(t, WarnIfEnvOTLP)
			if !strings.Contains(out, tt.want) {
				t.Fatalf("stderr = %q, want %q", out, tt.want)
			}
			if tt.want == "" && out != "" {
				t.Errorf("stderr = %q, want empty", out)
			}
		})
	}
}
