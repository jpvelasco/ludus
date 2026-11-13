package deploy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	deployapi "github.com/jpvelasco/ludus/internal/deploy"
)

func TestDestroyPreviewBinaryIsOffline(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "json"}[jsonOutput], func(t *testing.T) {
			checkBinaryPreview(t, jsonOutput)
		})
	}
}

func checkBinaryPreview(t *testing.T, jsonOutput bool) {
	t.Helper()
	t.Chdir(t.TempDir())
	cfg := config.Defaults()
	cfg.Deploy.Target, cfg.Deploy.OutputDir = "binary", "package"
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithJSONOutput(jsonOutput))
	saveDeployFlags(t)
	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deployapi.Target, error) {
		t.Fatal("offline preview constructed an execution target")
		return nil, nil
	})
	if err := os.Mkdir("package", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("package/server", []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(func() { runErr = runDestroy(newCommand(), nil) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	assertBinaryPreviewOutput(t, out, jsonOutput)
	got, err := os.ReadFile("package/server")
	if err != nil || string(got) != "keep" {
		t.Fatalf("preview changed output: %q, %v", got, err)
	}
}

func assertBinaryPreviewOutput(t *testing.T, out string, jsonOutput bool) {
	t.Helper()
	path, err := filepath.Abs("package")
	if err != nil {
		t.Fatal(err)
	}
	if jsonOutput {
		var plan map[string]any
		if err := json.Unmarshal([]byte(out), &plan); err != nil {
			t.Fatalf("preview is not one JSON plan: %v\n%s", err, out)
		}
		if plan["version"] != float64(1) {
			t.Fatalf("missing plan version: %v", plan)
		}
		return
	}
	for _, want := range []string{"Dry run — no AWS calls or changes made.", path, "recursive"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in preview:\n%s", want, out)
		}
	}
}
