package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
)

func TestHandleInitSummarizesPrerequisiteChecks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	previous := globals.Cfg
	cfg := config.Defaults()
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Game.ProjectPath = t.TempDir() + "/missing.uproject"
	globals.Cfg = cfg
	t.Cleanup(func() { globals.Cfg = previous })

	result, _, err := handleInit(context.Background(), nil, initInput{})
	if err != nil {
		t.Fatalf("handleInit: %v", err)
	}
	var got initResult
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(got.Checks) == 0 {
		t.Fatal("expected prerequisite checks")
	}
	if got.Passed+got.Failed+got.Warned != len(got.Checks) {
		t.Errorf("summary counts = %d, checks = %d", got.Passed+got.Failed+got.Warned, len(got.Checks))
	}
	if got.Success != (got.Failed == 0) {
		t.Errorf("success = %v with %d failed checks", got.Success, got.Failed)
	}
}
