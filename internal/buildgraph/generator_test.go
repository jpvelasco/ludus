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
	if !strings.Contains(serverNode.Steps[0].Arguments, "-serverplatform=Linux.LinuxArm64") {
		t.Errorf("BuildServer args should contain dependent ARM64 server platform, got %q", serverNode.Steps[0].Arguments)
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
	// FullBuild also aggregates the engine chain; the game nodes still let
	// runners reuse a prebuilt engine by requesting them directly (#704).
	if agg.Requires != "CompileEngine;BuildServer;BuildClient" {
		t.Errorf("aggregate requires: got %q, want CompileEngine;BuildServer;BuildClient", agg.Requires)
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
	if bg.Aggregates[0].Requires != "CompileEngine;BuildServer" {
		t.Errorf("aggregate requires: got %q, want CompileEngine;BuildServer", bg.Aggregates[0].Requires)
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

func TestDefaultMaxJobs(t *testing.T) {
	tt := []struct {
		in   int
		want int
	}{
		{0, 4},
		{8, 8},
		{2, 2},
	}
	for _, tc := range tt {
		if got := defaultMaxJobs(tc.in); got != tc.want {
			t.Errorf("defaultMaxJobs(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestGenerate_FullBuildIncludesEngineChain pins the #704 fix: requesting the
// FullBuild aggregate must reach the engine preparation chain, because a
// clean runner cannot package the game before the engine is set up and
// compiled.
func TestGenerate_FullBuildIncludesEngineChain(t *testing.T) {
	bg, err := Generate(defaultTestConfig(), "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reached := reachableNodes(bg, "FullBuild")
	for _, want := range []string{"CompileEngine", "GenerateProjectFiles", "Setup", "BuildServer"} {
		if !reached[want] {
			t.Errorf("FullBuild must reach node %q, reachable set: %v", want, reached)
		}
	}
}

// TestGenerate_GameNodesReuseExistingEngine pins the documented reuse
// semantics of #704: runners that already have a built engine request the
// game nodes directly, so those nodes must not depend on engine compilation.
func TestGenerate_GameNodesReuseExistingEngine(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ClientTarget = "LyraGame"

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gameAgent := requireAgent(t, bg, "Game")
	assertNodeRequires(t, requireNode(t, gameAgent, "BuildServer"), "")
	assertNodeRequires(t, requireNode(t, gameAgent, "BuildClient"), "")
}

// TestGenerate_PassesConfiguredTargets pins the #705 fix: the configured
// server and client targets must reach the BuildCookRun invocations so
// projects with multiple targets can select one.
func TestGenerate_PassesConfiguredTargets(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ServerTarget = "MyServerTarget"
	cfg.Game.ClientTarget = "MyClientTarget"

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertOption(t, bg, "ClientTarget", "MyClientTarget")
	gameAgent := requireAgent(t, bg, "Game")
	wants := []struct{ node, want string }{
		{"BuildServer", "-servertargetname=$(ServerTarget)"},
		{"BuildClient", "-clienttargetname=$(ClientTarget)"},
	}
	for _, tt := range wants {
		node := requireNode(t, gameAgent, tt.node)
		if !strings.Contains(node.Steps[0].Arguments, tt.want) {
			t.Errorf("%s args missing %q, got %q", tt.node, tt.want, node.Steps[0].Arguments)
		}
	}
}

// TestGenerate_QuotesPathArguments pins the #705 fix: every substituted
// filesystem argument is quoted so Spawn argument parsing keeps paths
// containing spaces as a single argument.
func TestGenerate_QuotesPathArguments(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Game.ProjectPath = "/workspace/Game Project/Lyra.uproject"
	cfg.Game.ClientTarget = "LyraGame"

	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gameAgent := requireAgent(t, bg, "Game")
	wants := []struct{ node, want string }{
		{"BuildServer", `-project="$(ProjectPath)"`},
		{"BuildServer", `-archivedirectory="/workspace/Game Project/PackagedServer"`},
		{"BuildClient", `-project="$(ProjectPath)"`},
		{"BuildClient", `-archivedirectory="/workspace/Game Project/PackagedClient"`},
	}
	for _, tt := range wants {
		node := requireNode(t, gameAgent, tt.node)
		if !strings.Contains(node.Steps[0].Arguments, tt.want) {
			t.Errorf("%s args missing %q, got %q", tt.node, tt.want, node.Steps[0].Arguments)
		}
	}
}

// reachableNodes resolves aggregate and node Requires edges (separated by
// semicolons) starting at the named aggregate and returns every node name
// reachable from it.
func reachableNodes(bg *BuildGraph, aggregateName string) map[string]bool {
	nodes := graphNodes(bg)
	reached := map[string]bool{}
	for _, agg := range bg.Aggregates {
		if agg.Name == aggregateName {
			for _, dep := range strings.Split(agg.Requires, ";") {
				visitNode(nodes, reached, dep)
			}
		}
	}
	return reached
}

// graphNodes indexes every node in the graph by name.
func graphNodes(bg *BuildGraph) map[string]*Node {
	nodes := map[string]*Node{}
	for _, agent := range bg.Agents {
		for i := range agent.Nodes {
			nodes[agent.Nodes[i].Name] = &agent.Nodes[i]
		}
	}
	return nodes
}

// visitNode marks a node reachable and recurses into its Requires edges.
func visitNode(nodes map[string]*Node, reached map[string]bool, name string) {
	if reached[name] {
		return
	}
	reached[name] = true
	node, ok := nodes[name]
	if !ok || node.Requires == "" {
		return
	}
	for _, dep := range strings.Split(node.Requires, ";") {
		visitNode(nodes, reached, dep)
	}
}
