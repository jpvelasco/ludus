package buildgraph

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func defaultTestConfig() *config.Config {
	cfg := config.Defaults()
	cfg.Engine.SourcePath = "/opt/unreal-engine"
	cfg.Game.ProjectPath = "/opt/unreal-engine/Samples/Games/Lyra/Lyra.uproject"
	cfg.Game.ProjectName = "Lyra"
	cfg.Game.Arch = "amd64"
	cfg.Game.ServerMap = "L_Expanse"
	cfg.Engine.MaxJobs = 8
	return cfg
}

// assertBuildGraphMatch verifies that two BuildGraph values produce identical XML.
func assertBuildGraphMatch(t *testing.T, want, got *BuildGraph) {
	t.Helper()
	wantData, err := want.Marshal()
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	gotData, err := got.Marshal()
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	if string(wantData) != string(gotData) {
		t.Errorf("BuildGraph mismatch after XML round trip:\ngot:  %s\nwant: %s", string(gotData), string(wantData))
	}
}

// assertOption verifies that the named option exists and has the expected default value.
func assertOption(t *testing.T, bg *BuildGraph, name, wantValue string) {
	t.Helper()
	opt := findOption(bg, name)
	if opt == nil {
		t.Errorf("option %q not found", name)
		return
	}
	if opt.DefaultValue != wantValue {
		t.Errorf("option %q = %q, want %q", name, opt.DefaultValue, wantValue)
	}
}

func findOption(bg *BuildGraph, name string) *Option {
	for i := range bg.Options {
		if bg.Options[i].Name == name {
			return &bg.Options[i]
		}
	}
	return nil
}

func findAgent(bg *BuildGraph, name string) *Agent {
	for i := range bg.Agents {
		if bg.Agents[i].Name == name {
			return &bg.Agents[i]
		}
	}
	return nil
}

func findNode(agent *Agent, name string) *Node {
	if agent == nil {
		return nil
	}
	for i := range agent.Nodes {
		if agent.Nodes[i].Name == name {
			return &agent.Nodes[i]
		}
	}
	return nil
}

func requireAgent(t *testing.T, bg *BuildGraph, name string) *Agent {
	t.Helper()
	agent := findAgent(bg, name)
	if agent == nil {
		t.Fatalf("agent %q not found", name)
	}
	return agent
}

func requireNode(t *testing.T, agent *Agent, name string) *Node {
	t.Helper()
	node := findNode(agent, name)
	if node == nil {
		t.Fatalf("node %q not found in agent %q", name, agent.Name)
	}
	return node
}

func assertProperty(t *testing.T, bg *BuildGraph, name, wantValue string) {
	t.Helper()
	for _, p := range bg.Properties {
		if p.Name == name {
			if p.Value != wantValue {
				t.Errorf("property %q = %q, want %q", name, p.Value, wantValue)
			}
			return
		}
	}
	t.Errorf("property %q not found", name)
}

func assertNodeRequires(t *testing.T, node *Node, want string) {
	t.Helper()
	if node.Requires != want {
		t.Errorf("node %q Requires = %q, want %q", node.Name, node.Requires, want)
	}
}

func assertAgentNodeCount(t *testing.T, agent *Agent, want int) {
	t.Helper()
	if len(agent.Nodes) != want {
		t.Fatalf("agent %q: want %d nodes, got %d", agent.Name, want, len(agent.Nodes))
	}
}

func assertStepCount(t *testing.T, node *Node, want int) {
	t.Helper()
	if len(node.Steps) != want {
		t.Fatalf("node %q: want %d steps, got %d", node.Name, want, len(node.Steps))
	}
}

func assertStep(t *testing.T, node *Node, index int, wantExe, wantArgs string) {
	t.Helper()
	if index >= len(node.Steps) {
		t.Fatalf("node %q: step %d out of range (have %d)", node.Name, index, len(node.Steps))
	}
	step := node.Steps[index]
	if step.Exe != wantExe {
		t.Errorf("node %q step %d exe = %q, want %q", node.Name, index, step.Exe, wantExe)
	}
	if step.Arguments != wantArgs {
		t.Errorf("node %q step %d args = %q, want %q", node.Name, index, step.Arguments, wantArgs)
	}
}

func assertStepArgs(t *testing.T, node *Node, index int, wantArgs string) {
	t.Helper()
	if index >= len(node.Steps) {
		t.Fatalf("node %q: step %d out of range (have %d)", node.Name, index, len(node.Steps))
	}
	if node.Steps[index].Arguments != wantArgs {
		t.Errorf("node %q step %d args = %q, want %q", node.Name, index, node.Steps[index].Arguments, wantArgs)
	}
}

func assertStepWorkingDir(t *testing.T, node *Node, index int, wantDir string) {
	t.Helper()
	if index >= len(node.Steps) {
		t.Fatalf("node %q: step %d out of range (have %d)", node.Name, index, len(node.Steps))
	}
	if node.Steps[index].WorkingDir != wantDir {
		t.Errorf("node %q step %d WorkingDir = %q, want %q", node.Name, index, node.Steps[index].WorkingDir, wantDir)
	}
}

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

	assertOption(t, bg, "Platform", "LinuxArm64")
	assertOption(t, bg, "Arch", "arm64")

	gameAgent := requireAgent(t, bg, "Game")
	serverNode := requireNode(t, gameAgent, "BuildServer")
	if !strings.Contains(serverNode.Steps[0].Arguments, "-platform=LinuxArm64") {
		t.Errorf("BuildServer args should contain -platform=LinuxArm64, got %q", serverNode.Steps[0].Arguments)
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

	// Verify XML header
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Error("XML output should start with XML declaration")
	}

	// Unmarshal back
	var bg2 BuildGraph
	if err := xml.Unmarshal(data, &bg2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify no data loss
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
