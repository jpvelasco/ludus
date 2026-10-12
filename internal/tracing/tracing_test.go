package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInit_DisabledIsNoop(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("Init(disabled) error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown func should never be nil")
	}
	// No-op shutdown must not error.
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown error = %v", err)
	}
}

func clearOTELEnv(t *testing.T) {
	t.Helper()
	for _, name := range OtelEnvVars {
		t.Setenv(name, "")
	}
}

func TestOtelEnvEnabled(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    bool
		wantSrc string
	}{
		{name: "no env", env: nil, want: false, wantSrc: ""},
		{name: "exporter none", env: map[string]string{"OTEL_TRACES_EXPORTER": "none"}, want: false, wantSrc: ""},
		{name: "exporter otlp", env: map[string]string{"OTEL_TRACES_EXPORTER": "otlp"}, want: true, wantSrc: "OTEL_TRACES_EXPORTER"},
		{name: "otlp endpoint", env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318"}, want: true, wantSrc: "OTEL_EXPORTER_OTLP_ENDPOINT"},
		{name: "traces endpoint", env: map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://c:4318"}, want: true, wantSrc: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearOTELEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := EnvEnabled(); got != tt.want {
				t.Errorf("EnvEnabled() = %v, want %v", got, tt.want)
			}
			if got := EnvSource(); got != tt.wantSrc {
				t.Errorf("EnvSource() = %q, want %q", got, tt.wantSrc)
			}
		})
	}
}

func TestResolvedOTLPEndpoint(t *testing.T) {
	clearOTELEnv(t)

	t.Run("config endpoint wins", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env:4318")
		if got := ResolvedOTLPEndpoint("config:4318"); got != "config:4318" {
			t.Errorf("ResolvedOTLPEndpoint() = %q, want config endpoint", got)
		}
	})
	t.Run("env endpoint when no config", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env:4318")
		if got := ResolvedOTLPEndpoint(""); got != "http://env:4318" {
			t.Errorf("ResolvedOTLPEndpoint() = %q, want env endpoint", got)
		}
	})
	t.Run("traces endpoint wins over general env endpoint", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://general:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://traces:4318")
		if got := ResolvedOTLPEndpoint(""); got != "http://traces:4318" {
			t.Errorf("ResolvedOTLPEndpoint() = %q, want traces endpoint", got)
		}
	})
	t.Run("SDK default when neither", func(t *testing.T) {
		if got := ResolvedOTLPEndpoint(""); got != defaultOTLPEndpoint {
			t.Errorf("ResolvedOTLPEndpoint() = %q, want %q", got, defaultOTLPEndpoint)
		}
	})
}

func TestDiagnose(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		env  map[string]string
		want OtlpState
	}{
		{name: "disabled, no env", cfg: Config{}, env: nil, want: OtlpState{}},
		{name: "config enabled", cfg: Config{Enabled: true, Endpoint: "c:4318"}, env: nil,
			want: OtlpState{Active: true, Source: "config", Endpoint: "c:4318"}},
		{name: "config enabled defaults endpoint", cfg: Config{Enabled: true}, env: nil,
			want: OtlpState{Active: true, Source: "config", Endpoint: defaultOTLPEndpoint}},
		{name: "env only", cfg: Config{}, env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://e:4318"},
			want: OtlpState{Active: true, Source: "OTEL_EXPORTER_OTLP_ENDPOINT", Endpoint: "http://e:4318"}},
		{name: "config and env, config wins", cfg: Config{Enabled: true, Endpoint: "c:4318"},
			env:  map[string]string{"OTEL_TRACES_EXPORTER": "otlp"},
			want: OtlpState{Active: true, Source: "config", Endpoint: "c:4318"}},
		{name: "exporter none does not opt in", cfg: Config{}, env: map[string]string{"OTEL_TRACES_EXPORTER": "none"},
			want: OtlpState{}},
		{name: "signal-specific env endpoint wins", cfg: Config{},
			env: map[string]string{
				"OTEL_TRACES_EXPORTER":               "otlp",
				"OTEL_EXPORTER_OTLP_ENDPOINT":        "http://general:4318",
				"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://traces:4318",
			},
			want: OtlpState{Active: true, Source: "OTEL_TRACES_EXPORTER", Endpoint: "http://traces:4318"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearOTELEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := Diagnose(tt.cfg); got != tt.want {
				t.Errorf("Diagnose() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTracer_RecordsSpan(t *testing.T) {
	// Install an in-memory recorder so we can assert span creation without a
	// collector or network.
	rec := tracetest.NewSpanRecorder()
	restore := UseRecorder(rec)
	defer restore()

	ctx, span := Tracer().Start(context.Background(), "test-stage")
	span.End()
	_ = ctx

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 recorded span, got %d", len(spans))
	}
	if spans[0].Name() != "test-stage" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "test-stage")
	}
}

func TestExporterOptions(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{name: "defaults", cfg: Config{}, want: 0},
		{name: "endpoint", cfg: Config{Endpoint: "collector:4318"}, want: 1},
		{name: "insecure", cfg: Config{Insecure: true}, want: 1},
		{name: "headers", cfg: Config{Headers: map[string]string{"api-key": "secret"}}, want: 1},
		{name: "all", cfg: Config{Endpoint: "collector:4318", Insecure: true, Headers: map[string]string{"api-key": "secret"}}, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(exporterOptions(tt.cfg)); got != tt.want {
				t.Errorf("len(exporterOptions()) = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBuildResource(t *testing.T) {
	for _, version := range []string{"1.2.3", ""} {
		if got := buildResource(version); got == nil {
			t.Errorf("buildResource(%q) returned nil", version)
		}
	}
}
func TestInit_EnabledConfiguresProvider(t *testing.T) {
	for _, key := range []string{"OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		t.Setenv(key, "")
	}
	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	shutdown, err := Init(context.Background(), Config{Enabled: true, Endpoint: "127.0.0.1:4318", Insecure: true, Version: "test"})
	if err != nil {
		t.Fatalf("Init(enabled) error = %v", err)
	}
	if otel.GetTracerProvider() == previous {
		t.Error("Init(enabled) did not install a tracer provider")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown error = %v", err)
	}
}
