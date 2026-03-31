package doctor

import (
	"testing"
)

func TestCountDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		checks    []diagnostic
		wantFails int
		wantWarns int
	}{
		{
			name:      "no checks",
			checks:    nil,
			wantFails: 0,
			wantWarns: 0,
		},
		{
			name: "all ok",
			checks: []diagnostic{
				{name: "a", status: "ok"},
				{name: "b", status: "ok"},
				{name: "c", status: "ok"},
			},
			wantFails: 0,
			wantWarns: 0,
		},
		{
			name: "mixed fails warns and ok",
			checks: []diagnostic{
				{name: "a", status: "fail"},
				{name: "b", status: "warn"},
				{name: "c", status: "ok"},
				{name: "d", status: "fail"},
				{name: "e", status: "warn"},
				{name: "f", status: "warn"},
			},
			wantFails: 2,
			wantWarns: 3,
		},
		{
			name: "only fails",
			checks: []diagnostic{
				{name: "a", status: "fail"},
				{name: "b", status: "fail"},
			},
			wantFails: 2,
			wantWarns: 0,
		},
		{
			name: "only warns",
			checks: []diagnostic{
				{name: "a", status: "warn"},
			},
			wantFails: 0,
			wantWarns: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFails, gotWarns := countDiagnostics(tt.checks)
			if gotFails != tt.wantFails {
				t.Errorf("fails = %d, want %d", gotFails, tt.wantFails)
			}
			if gotWarns != tt.wantWarns {
				t.Errorf("warns = %d, want %d", gotWarns, tt.wantWarns)
			}
		})
	}
}

func TestDiagnosticMarker(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "ok", status: "ok", want: "[OK]  "},
		{name: "warn", status: "warn", want: "[WARN]"},
		{name: "fail", status: "fail", want: "[FAIL]"},
		{name: "unknown falls through to default", status: "something", want: "[OK]  "},
		{name: "empty falls through to default", status: "", want: "[OK]  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diagnosticMarker(tt.status)
			if got != tt.want {
				t.Errorf("diagnosticMarker(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestFormatDiagnosticSummary(t *testing.T) {
	tests := []struct {
		name    string
		fails   int
		warns   int
		wantErr bool
	}{
		{
			name:    "no issues",
			fails:   0,
			warns:   0,
			wantErr: false,
		},
		{
			name:    "warnings only",
			fails:   0,
			warns:   3,
			wantErr: false,
		},
		{
			name:    "failures only",
			fails:   2,
			warns:   0,
			wantErr: true,
		},
		{
			name:    "both failures and warnings",
			fails:   1,
			warns:   2,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := formatDiagnosticSummary(tt.fails, tt.warns)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
		})
	}
}
