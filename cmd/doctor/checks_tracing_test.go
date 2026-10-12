package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/tracing"
)

func clearOTELEnv(t *testing.T) {
	t.Helper()
	for _, k := range tracing.OtelEnvVars {
		t.Setenv(k, "")
	}
}

type otlpCheckCase struct {
	name         string
	cfg          *config.Config
	env          map[string]string
	wantStatus   string
	wantMessage  string
	wantEndpoint string
}

func otlpCheckCases() []otlpCheckCase {
	return []otlpCheckCase{
		{
			name:        "disabled by default",
			cfg:         &config.Config{},
			wantStatus:  "ok",
			wantMessage: "off",
		},
		{
			name:        "nil config with no env",
			cfg:         nil,
			wantStatus:  "ok",
			wantMessage: "off",
		},
		{
			name:         "config enabled",
			cfg:          &config.Config{Observability: config.ObservabilityConfig{OTLP: config.OTLPConfig{Enabled: true, Endpoint: "collector:4318"}}},
			wantStatus:   "ok",
			wantMessage:  "on (observability.otlp.enabled=true)",
			wantEndpoint: "collector:4318",
		},
		{
			name:         "config enabled without endpoint falls back to default",
			cfg:          &config.Config{Observability: config.ObservabilityConfig{OTLP: config.OTLPConfig{Enabled: true}}},
			wantStatus:   "ok",
			wantMessage:  "on (observability.otlp.enabled=true)",
			wantEndpoint: "localhost:4318",
		},
		{
			name:         "env enabled via endpoint var",
			cfg:          &config.Config{},
			env:          map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://org-collector:4318"},
			wantStatus:   "ok",
			wantMessage:  "on (via OTEL_EXPORTER_OTLP_ENDPOINT)",
			wantEndpoint: "http://org-collector:4318",
		},
		{
			name:        "exporter none does not enable",
			cfg:         &config.Config{},
			env:         map[string]string{"OTEL_TRACES_EXPORTER": "none"},
			wantStatus:  "ok",
			wantMessage: "off",
		},
	}
}

func TestCheckOTLPTracing(t *testing.T) {
	for _, tt := range otlpCheckCases() {
		t.Run(tt.name, func(t *testing.T) {
			clearOTELEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			assertOTLPDiag(t, checkOTLPTracing(tt.cfg), tt.wantStatus, tt.wantMessage, tt.wantEndpoint)
		})
	}
}

// assertOTLPDiag verifies one OTLP diagnostic's status, message, and endpoint.
func assertOTLPDiag(t *testing.T, d diagnostic, wantStatus, wantMessage, wantEndpoint string) {
	t.Helper()
	if d.status != wantStatus {
		t.Errorf("status = %q, want %q", d.status, wantStatus)
	}
	if !strings.Contains(d.message, wantMessage) {
		t.Errorf("message = %q, want substring %q", d.message, wantMessage)
	}
	if wantEndpoint == "" {
		return
	}
	if len(d.details) != 1 || !strings.Contains(d.details[0], wantEndpoint) {
		t.Errorf("details = %v, want endpoint %q", d.details, wantEndpoint)
	}
}

// assertJSONResult verifies the expected fields of one decoded diagnostic
// result.
func assertJSONResult(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	for key, wantVal := range want {
		if got[key] != wantVal {
			t.Errorf("result[%q] = %v, want %v", key, got[key], wantVal)
		}
	}
}

// jsonFieldString renders a decoded diagnostic field as a string: top-level
// strings as-is, details slices joined.
func jsonFieldString(result map[string]any, field string) string {
	if v, ok := result[field].(string); ok {
		return v
	}
	arr, ok := result[field].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, len(arr))
	for i, e := range arr {
		parts[i] = fmt.Sprint(e)
	}
	return strings.Join(parts, " ")
}

// assertFieldContains verifies a decoded field renders as a string containing
// the given substring.
func assertFieldContains(t *testing.T, result map[string]any, field, want string) {
	t.Helper()
	got := jsonFieldString(result, field)
	if !strings.Contains(got, want) {
		t.Errorf("field %q = %q, want substring %q", field, got, want)
	}
}

func TestPrintDiagnosticsJSON(t *testing.T) {
	output, runErr := captureStdout(t, func() error {
		return printDiagnosticsJSON([]diagnostic{
			{name: "OTLP Tracing", status: "ok", message: "on (via OTEL_EXPORTER_OTLP_ENDPOINT)", details: []string{"endpoint: http://e:4318"}},
			{name: "Bad", status: "fail", message: "broken"},
		})
	})

	if runErr == nil || !strings.Contains(runErr.Error(), "1 diagnostic check(s) failed") {
		t.Errorf("printDiagnosticsJSON() error = %v, want failure error", runErr)
	}

	var results []map[string]any
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("output %q is not valid JSON: %v", output, err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	assertJSONResult(t, results[0], map[string]any{"name": "OTLP Tracing", "status": "ok"})
	assertJSONResult(t, results[1], map[string]any{"status": "fail"})
	assertFieldContains(t, results[0], "message", "OTEL_EXPORTER_OTLP_ENDPOINT")
	assertFieldContains(t, results[0], "details", "http://e:4318")
}

func TestPrintDiagnosticsJSONClean(t *testing.T) {
	output, runErr := captureStdout(t, func() error {
		return printDiagnosticsJSON([]diagnostic{{name: "ok check", status: "ok", message: "fine"}})
	})

	if runErr != nil {
		t.Fatalf("printDiagnosticsJSON() error = %v, want nil", runErr)
	}
	if !strings.Contains(output, "ok check") {
		t.Errorf("output %q missing check name", output)
	}
}
