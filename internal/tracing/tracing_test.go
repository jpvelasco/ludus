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
