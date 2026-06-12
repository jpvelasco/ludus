package buildgraph

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestGenerate_AMD64(t *testing.T) {
	cfg := defaultTestConfig()
	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertOption(t, bg, "SourcePath", "/opt/unreal-engine")
	assertOption(t, bg, "ProjectPath", cfg.Game.ProjectPath)
	assertOption(t, bg, "ProjectName", "Lyra")
	assertOption(t, bg, "ServerTarget", "LyraServer")
	assertOption(t, bg, "Platform", "Linux")
	assertOption(t, bg, "Arch", "amd64")
	assertOption(t, bg, "MaxJobs", "8")
	assertOption(t, bg, "ServerConfig", "Development")
	assertProperty(t, bg, "EngineVersion", "5.7.3")

	engineAgent := requireAgent(t, bg, "Engine")
	assertAgentNodeCount(t, engineAgent, 3)

	setupNode := requireNode(t, engineAgent, "Setup")
	assertNodeRequires(t, setupNode, "")
	assertStepCount(t, setupNode, 1)
	assertStep(t, setupNode, 0, "bash", "Setup.sh")
	assertStepWorkingDir(t, setupNode, 0, "$(SourcePath)")

	gpfNode := requireNode(t, engineAgent, "GenerateProjectFiles")
	assertNodeRequires(t, gpfNode, "Setup")

	compileNode := requireNode(t, engineAgent, "CompileEngine")
	assertNodeRequires(t, compileNode, "GenerateProjectFiles")
	assertStepCount(t, compileNode, 2)
	assertStepArgs(t, compileNode, 0, "-j$(MaxJobs) ShaderCompileWorker")
	assertStepArgs(t, compileNode, 1, "-j$(MaxJobs) UnrealEditor")

	gameAgent := requireAgent(t, bg, "Game")
	assertAgentNodeCount(t, gameAgent, 1)
	if gameAgent.Nodes[0].Name != "BuildServer" {
		t.Errorf("Game node name: got %q, want BuildServer", gameAgent.Nodes[0].Name)
	}
}

func TestGenerate_ARM64(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.Arch = "arm64"

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertOption(t, bg, "Platform", "Linux")
	assertOption(t, bg, "Arch", "arm64")

	gameAgent := requireAgent(t, bg, "Game")
	serverNode := requireNode(t, gameAgent, "BuildServer")
	if !strings.Contains(serverNode.Steps[0].Arguments, "-platform=Linux") {
		t.Errorf("BuildServer args should contain -platform=Linux, got %q", serverNode.Steps[0].Arguments)
	}
}

func TestGenerate_WithClient(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ClientTarget = "LyraGame"

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gameAgent := requireAgent(t, bg, "Game")
	assertAgentNodeCount(t, gameAgent, 2)
	requireNode(t, gameAgent, "BuildServer")
	requireNode(t, gameAgent, "BuildClient")

	if len(bg.Aggregates) != 1 {
		t.Fatalf("want 1 aggregate, got %d", len(bg.Aggregates))
	}
	agg := bg.Aggregates[0]
	if agg.Name != "FullBuild" {
		t.Errorf("aggregate name: got %q, want FullBuild", agg.Name)
	}
	if agg.Requires != "BuildServer;BuildClient" {
		t.Errorf("aggregate requires: got %q, want BuildServer;BuildClient", agg.Requires)
	}
}

func TestGenerate_WithoutClient(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ClientTarget = ""

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gameAgent := requireAgent(t, bg, "Game")
	assertAgentNodeCount(t, gameAgent, 1)

	if len(bg.Aggregates) != 1 {
		t.Fatalf("want 1 aggregate, got %d", len(bg.Aggregates))
	}
	if bg.Aggregates[0].Requires != "BuildServer" {
		t.Errorf("aggregate requires: got %q, want BuildServer", bg.Aggregates[0].Requires)
	}
}

func TestGenerate_Shipping(t *testing.T) {
	cfg := defaultTestConfig()

	bg, err := Generate(cfg, "5.7.3", WithServerConfig("Shipping"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opt := findOption(bg, "ServerConfig")
	if opt == nil {
		t.Fatal("ServerConfig option not found")
		return
	}
	if opt.DefaultValue != "Shipping" {
		t.Errorf("ServerConfig default: got %q, want Shipping", opt.DefaultValue)
	}
}

func TestGenerate_XMLRoundTrip(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ClientTarget = "LyraGame"

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := bg.Marshal()
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if !strings.HasPrefix(string(data), "<?xml") {
		t.Error("XML output should start with XML declaration")
	}

	var bg2 BuildGraph
	if err := xml.Unmarshal(data, &bg2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	assertBuildGraphMatch(t, bg, &bg2)
}

func TestGenerate_MissingSourcePath(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Engine.SourcePath = ""

	_, err := Generate(cfg, "5.7.3")
	if err == nil {
		t.Fatal("expected error for missing source path")
	}
	if !strings.Contains(err.Error(), "source path") {
		t.Errorf("error should mention source path, got: %v", err)
	}
}

func TestGenerate_MissingProjectPath(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ProjectPath = ""

	_, err := Generate(cfg, "5.7.3")
	if err == nil {
		t.Fatal("expected error for missing project path")
	}
	if !strings.Contains(err.Error(), "project path") {
		t.Errorf("error should mention project path, got: %v", err)
	}
}
