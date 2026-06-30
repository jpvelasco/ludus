package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/dflint"
	"github.com/jpvelasco/ludus/internal/state"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHandleConnectInfo(t *testing.T) {
	t.Chdir(t.TempDir())
	previousProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previousProfile) })

	saved := &state.State{
		Session: &state.SessionState{
			SessionID: "session-1",
			IPAddress: "203.0.113.10",
			Port:      7777,
		},
		Client: &state.ClientState{
			BinaryPath: "/build/LyraClient",
			Platform:   "Linux",
		},
	}
	if err := state.Save(saved); err != nil {
		t.Fatal(err)
	}

	result, _, err := handleConnectInfo(context.Background(), nil, connectInput{})
	if err != nil {
		t.Fatal(err)
	}
	text := toolResultText(t, result)
	for _, want := range []string{"session-1", "203.0.113.10:7777", "/build/LyraClient", "Linux"} {
		if !strings.Contains(text, want) {
			t.Errorf("result %q does not contain %q", text, want)
		}
	}
}

func TestHandleConnectInfoEmptyState(t *testing.T) {
	t.Chdir(t.TempDir())
	previousProfile := state.ActiveProfile()
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile(previousProfile) })

	result, _, err := handleConnectInfo(context.Background(), nil, connectInput{})
	if err != nil {
		t.Fatal(err)
	}
	if text := toolResultText(t, result); text != "{}" {
		t.Fatalf("result = %q, want empty object", text)
	}
}

func TestHandleBuildGraph(t *testing.T) {
	previous := globals.Cfg
	cfg := config.Defaults()
	cfg.Engine.SourcePath = "/opt/unreal-engine"
	cfg.Engine.Version = "5.7.3"
	cfg.Game.ProjectPath = "/projects/Lyra/Lyra.uproject"
	cfg.Game.ProjectName = "Lyra"
	globals.Cfg = cfg
	t.Cleanup(func() { globals.Cfg = previous })

	result, _, err := handleBuildGraph(context.Background(), nil, buildGraphInput{})
	if err != nil {
		t.Fatal(err)
	}
	text := toolResultText(t, result)
	for _, want := range []string{"<BuildGraph", "Lyra.uproject", "EngineVersion"} {
		if !strings.Contains(text, want) {
			t.Errorf("result does not contain %q", want)
		}
	}
}

func TestLintResultToScan(t *testing.T) {
	result := &dflint.LintResult{
		Findings: []dflint.Finding{
			{Source: "hadolint", Rule: "DL3006", Level: dflint.SeverityWarning, Message: "pin image", Line: 4},
		},
	}
	scan := lintResultToScan("Dockerfile", result)
	if scan.Target != "Dockerfile" || len(scan.Findings) != 1 {
		t.Fatalf("scan = %+v", scan)
	}
	got := scan.Findings[0]
	if got.Rule != "DL3006" || got.Level != string(dflint.SeverityWarning) || got.Line != 4 {
		t.Fatalf("finding = %+v", got)
	}
}

func toolResultText(t *testing.T, result *sdkmcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}
