package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestDestroyPreviewMCPDryRunAndScopeAxes(t *testing.T) {
	for _, mode := range []struct {
		name          string
		input, global bool
	}{
		{"input", true, false}, {"global", false, true}, {"both", true, true},
	} {
		for _, scope := range []struct{ all, purge bool }{{}, {true, false}, {false, true}, {true, true}} {
			t.Run(fmt.Sprintf("%s/all=%t/purge=%t", mode.name, scope.all, scope.purge), func(t *testing.T) {
				checkMCPDestroyPreview(t, deployDestroyInput{DryRun: mode.input, AllTargets: scope.all, Purge: scope.purge}, mode.global)
			})
		}
	}
}

func checkMCPDestroyPreview(t *testing.T, input deployDestroyInput, global bool) {
	t.Helper()
	t.Chdir(t.TempDir())
	previous := state.ActiveProfile()
	state.SetProfile("preview")
	t.Cleanup(func() { state.SetProfile(previous) })
	cfg := config.Defaults()
	cfg.Deploy.Target, cfg.AWS.AccountID = "binary", "123456789012"
	globals.SetGlobals(t, cfg, globals.WithDryRun(global))
	before := cfg.Clone()
	globals.SwapResolveTarget(t, func(context.Context, *config.Config, string) (deploy.Target, error) {
		t.Fatal("MCP preview resolved an execution target")
		return nil, nil
	})
	result, _, err := handleDeployDestroy(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	var payload deployDestroyResult
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &payload); err != nil {
		t.Fatal(err)
	}
	assertMCPPreviewScope(t, payload, input)
	if !reflect.DeepEqual(before, *cfg) {
		t.Fatal("MCP preview changed configuration")
	}
}

func assertMCPPreviewScope(t *testing.T, payload deployDestroyResult, input deployDestroyInput) {
	t.Helper()
	if !payload.Success || payload.Plan == nil {
		t.Fatalf("missing successful plan: %+v", payload)
	}
	targets, artifacts := 1, 0
	if input.AllTargets {
		targets = 5
	}
	if input.Purge {
		artifacts = 2
	}
	for name, ok := range map[string]bool{
		"targets":           len(payload.Plan.Targets) == targets,
		"purge":             len(payload.Plan.Purge) == artifacts,
		"profile":           payload.Plan.Profile == "preview",
		"raw identity":      payload.Plan.ConfiguredAccountID == "123456789012",
		"shared transcript": payload.Output == deploy.RenderDestroyPlan(payload.Plan),
	} {
		if !ok {
			t.Errorf("%s differs in %+v", name, payload)
		}
	}
}

func TestDestroyPreviewMCPReportsMalformedState(t *testing.T) {
	t.Chdir(t.TempDir())
	previous := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previous) })
	globals.SetGlobals(t, config.Defaults())
	if err := os.Mkdir(".ludus", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".ludus", "state.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, _, err := handleDeployDestroy(context.Background(), nil, deployDestroyInput{DryRun: true})
	if err != nil || !result.IsError || !strings.Contains(toolResultText(t, result), "loading destroy preview state") {
		t.Fatalf("malformed state result = %v, %v", result, err)
	}
}

func TestDestroyPreviewMCPReportsIncompleteSweep(t *testing.T) {
	t.Chdir(t.TempDir())
	globals.SetGlobals(t, &config.Config{})
	result, _, err := handleDeployDestroy(context.Background(), nil, deployDestroyInput{DryRun: true, AllTargets: true})
	if err != nil {
		t.Fatal(err)
	}
	var payload deployDestroyResult
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &payload); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || payload.Plan == nil || len(payload.Plan.Warnings) != 2 || len(payload.Plan.Targets) != 3 {
		t.Fatalf("missing partial plan/error: %+v", payload)
	}
}
