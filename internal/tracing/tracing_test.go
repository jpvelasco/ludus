package tracing

import (
	"context"
	"testing"

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

func TestOtelEnvEnabled(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "no env", env: nil, want: false},
		{name: "exporter none", env: map[string]string{"OTEL_TRACES_EXPORTER": "none"}, want: false},
		{name: "exporter otlp", env: map[string]string{"OTEL_TRACES_EXPORTER": "otlp"}, want: true},
		{name: "otlp endpoint", env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318"}, want: true},
		{name: "traces endpoint", env: map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://c:4318"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant vars, then set the case's.
			for _, k := range []string{"OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := otelEnvEnabled(); got != tt.want {
				t.Errorf("otelEnvEnabled() = %v, want %v", got, tt.want)
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
