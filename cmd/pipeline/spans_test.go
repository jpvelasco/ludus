package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/jpvelasco/ludus/internal/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestExecuteStages_EmitsSpanPerStage(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	restore := tracing.UseRecorder(rec)
	defer restore()

	stages := []pipelineStage{
		{name: "Stage A", fn: func(context.Context) error { return nil }},
		{name: "Stage B (skipped)", skip: true, fn: func(context.Context) error { return nil }},
		{name: "Stage C", fn: func(context.Context) error { return nil }},
	}

	if err := executeStages(context.Background(), stages); err != nil {
		t.Fatalf("executeStages error = %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (skipped stage emits none), got %d", len(spans))
	}
	names := []string{spans[0].Name(), spans[1].Name()}
	for _, want := range []string{"Stage A", "Stage C"} {
		if !contains(names, want) {
			t.Errorf("missing span %q; got %v", want, names)
		}
	}
}

func TestExecuteStages_RecordsErrorOnFailedStage(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	restore := tracing.UseRecorder(rec)
	defer restore()

	wantErr := errors.New("boom")
	stages := []pipelineStage{
		{name: "Failing", fn: func(context.Context) error { return wantErr }},
	}

	if err := executeStages(context.Background(), stages); err == nil {
		t.Fatal("expected error from failing stage")
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code != otelcodes.Error {
		t.Errorf("span status = %v, want Error", spans[0].Status().Code)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
