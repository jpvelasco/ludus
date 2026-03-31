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

func TestGenerate_AMD64(t *testing.T) {
	cfg := defaultTestConfig()
	bg, err := Generate(cfg, "5.7.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify options
	assertOption(t, bg, "SourcePath", "/opt/unreal-engine")
	assertOption(t, bg, "ProjectPath", cfg.Game.ProjectPath)
	assertOption(t, bg, "ProjectName", "Lyra")
	assertOption(t, bg, "ServerTarget", "LyraServer")
	assertOption(t, bg, "Platform", "Linux")
	assertOption(t, bg, "Arch", "amd64")
	assertOption(t, bg, "MaxJobs", "8")
	assertOption(t, bg, "ServerConfig", "Development")

	// Verify EngineVersion property
	if len(bg.Properties) != 1 || bg.Properties[0].Name != "EngineVersion" || bg.Properties[0].Value != "5.7.3" {
		t.Errorf("EngineVersion property: got %+v", bg.Properties)
	}

	// Verify Engine agent with 3 nodes
	engineAgent := findAgent(bg, "Engine")
	if engineAgent == nil {
		t.Fatal("Engine agent not found")
		return
	}
	if len(engineAgent.Nodes) != 3 {
		t.Fatalf("Engine agent: want 3 nodes, got %d", len(engineAgent.Nodes))
	}

	setupNode := findNode(engineAgent, "Setup")
	if setupNode == nil {
		t.Fatal("Setup node not found")
		return
	}
	if setupNode.Requires != "" {
		t.Errorf("Setup node should have no Requires, got %q", setupNode.Requires)
	}
	if len(setupNode.Steps) != 1 || setupNode.Steps[0].Exe != "bash" || setupNode.Steps[0].Arguments != "Setup.sh" {
		t.Errorf("Setup step: got %+v", setupNode.Steps)
	}
	if setupNode.Steps[0].WorkingDir != "$(SourcePath)" {
		t.Errorf("Setup WorkingDir: got %q, want $(SourcePath)", setupNode.Steps[0].WorkingDir)
	}

	gpfNode := findNode(engineAgent, "GenerateProjectFiles")
	if gpfNode == nil {
		t.Fatal("GenerateProjectFiles node not found")
		return
	}
	if gpfNode.Requires != "Setup" {
		t.Errorf("GenerateProjectFiles Requires: got %q, want Setup", gpfNode.Requires)
	}

	compileNode := findNode(engineAgent, "CompileEngine")
	if compileNode == nil {
		t.Fatal("CompileEngine node not found")
		return
	}
	if compileNode.Requires != "GenerateProjectFiles" {
		t.Errorf("CompileEngine Requires: got %q, want GenerateProjectFiles", compileNode.Requires)
	}
	if len(compileNode.Steps) != 2 {
		t.Fatalf("CompileEngine: want 2 steps, got %d", len(compileNode.Steps))
	}
	if compileNode.Steps[0].Arguments != "-j$(MaxJobs) ShaderCompileWorker" {
		t.Errorf("CompileEngine step 0 args: got %q", compileNode.Steps[0].Arguments)
	}
	if compileNode.Steps[1].Arguments != "-j$(MaxJobs) UnrealEditor" {
		t.Errorf("CompileEngine step 1 args: got %q", compileNode.Steps[1].Arguments)
	}

	// Verify Game agent with 1 node (no client)
	gameAgent := findAgent(bg, "Game")
	if gameAgent == nil {
		t.Fatal("Game agent not found")
		return
	}
	if len(gameAgent.Nodes) != 1 {
		t.Fatalf("Game agent: want 1 node, got %d", len(gameAgent.Nodes))
	}
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

	if opt := findOption(bg, "Platform"); opt == nil || opt.DefaultValue != "LinuxArm64" {
		t.Errorf("Platform option: got %+v, want DefaultValue=LinuxArm64", opt)
	}
	if opt := findOption(bg, "Arch"); opt == nil || opt.DefaultValue != "arm64" {
		t.Errorf("Arch option: got %+v, want DefaultValue=arm64", opt)
	}

	// Verify the server build args use LinuxArm64 platform
	gameAgent := findAgent(bg, "Game")
	if gameAgent == nil {
		t.Fatal("Game agent not found")
		return
	}
	serverNode := findNode(gameAgent, "BuildServer")
	if serverNode == nil {
		t.Fatal("BuildServer node not found")
		return
	}
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

	gameAgent := findAgent(bg, "Game")
	if gameAgent == nil {
		t.Fatal("Game agent not found")
		return
	}
	if len(gameAgent.Nodes) != 2 {
		t.Fatalf("Game agent with client: want 2 nodes, got %d", len(gameAgent.Nodes))
	}

	serverNode := findNode(gameAgent, "BuildServer")
	if serverNode == nil {
		t.Error("BuildServer node not found")
	}
	clientNode := findNode(gameAgent, "BuildClient")
	if clientNode == nil {
		t.Error("BuildClient node not found")
	}

	// Aggregate should require both
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

	gameAgent := findAgent(bg, "Game")
	if gameAgent == nil {
		t.Fatal("Game agent not found")
		return
	}
	if len(gameAgent.Nodes) != 1 {
		t.Fatalf("Game agent without client: want 1 node, got %d", len(gameAgent.Nodes))
	}

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
