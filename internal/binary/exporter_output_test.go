package binary

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/deploy"
)

// TestSetOutputRoutesDestroyNote pins #765: after SetOutput, the exporter's
// removal note lands in the routed writer (an MCP destroy sweep's
// per-operation capture) instead of the process stdout.
func TestSetOutputRoutesDestroyNote(t *testing.T) {
	dir := t.TempDir()
	e := NewExporter(dir)

	var buf bytes.Buffer
	e.SetOutput(&buf)

	if err := e.Destroy(context.Background()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Removed ") {
		t.Errorf("routed output = %q, want the removal note", buf.String())
	}
}

// TestExporterImplementsOutputSetter pins the wiring contract: the exporter is
// its own deploy.Target and must satisfy deploy.OutputSetter so the MCP
// destroy sweep can route its capture (#765).
func TestExporterImplementsOutputSetter(t *testing.T) {
	var target deploy.Target = NewExporter(t.TempDir())
	if _, ok := target.(deploy.OutputSetter); !ok {
		t.Error("Exporter does not implement deploy.OutputSetter")
	}
}
