package mcp

import (
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestJSONstringReportsMarshalError(t *testing.T) {
	got := jsonString(make(chan int))
	if !strings.Contains(got, "unsupported type") {
		t.Fatalf("jsonString error = %q", got)
	}
}

func TestMergeOutput(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		stderr string
		want   string
	}{
		{"both present", "out", "err", "outerr"},
		{"stdout only", "output", "", "output"},
		{"stderr only", "", "error", "error"},
		{"both empty", "", "", ""},
		{"multiline", "line1\nline2\n", "warn\n", "line1\nline2\nwarn\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := capturedOutput{Stdout: tt.stdout, Stderr: tt.stderr}
			got := mergeOutput(c)
			if got != tt.want {
				t.Errorf("mergeOutput(%q, %q) = %q, want %q", tt.stdout, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestResultOK(t *testing.T) {
	type payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	v := payload{Success: true, Message: "done"}

	result, structured, err := resultOK(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Errorf("expected nil structured result, got %v", structured)
	}
	if result.IsError {
		t.Error("expected IsError = false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected *mcpsdk.TextContent, got %T", result.Content[0])
	}
	if tc.Text == "" {
		t.Error("expected non-empty text content")
	}
}

func TestResultErr(t *testing.T) {
	type payload struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	v := payload{Success: false, Error: "something failed"}

	result, structured, err := resultErr(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structured != nil {
		t.Errorf("expected nil structured result, got %v", structured)
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
}
