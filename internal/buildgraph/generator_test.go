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
	if opt := findOption(bg, "SourcePath"); opt == nil || opt.DefaultValue != "/opt/unreal-engine" {
		t.Errorf("SourcePath option: got %+v", opt)
	}
	if opt := findOption(bg, "ProjectPath"); opt == nil || opt.DefaultValue != cfg.Game.ProjectPath {
		t.Errorf("ProjectPath option: got %+v", opt)
	}
	if opt := findOption(bg, "ProjectName"); opt == nil || opt.DefaultValue != "Lyra" {
		t.Errorf("ProjectName option: got %+v", opt)
	}
	if opt := findOption(bg, "ServerTarget"); opt == nil || opt.DefaultValue != "LyraServer" {
		t.Errorf("ServerTarget option: got %+v", opt)
	}
	if opt := findOption(bg, "Platform"); opt == nil || opt.DefaultValue != "Linux" {
		t.Errorf("Platform option: got %+v, want DefaultValue=Linux", opt)
	}
	if opt := findOption(bg, "Arch"); opt == nil || opt.DefaultValue != "amd64" {
		t.Errorf("Arch option: got %+v", opt)
	}
	if opt := findOption(bg, "MaxJobs"); opt == nil || opt.DefaultValue != "8" {
		t.Errorf("MaxJobs option: got %+v, want DefaultValue=8", opt)
	}
	if opt := findOption(bg, "ServerConfig"); opt == nil || opt.DefaultValue != "Development" {
		t.Errorf("ServerConfig option: got %+v, want DefaultValue=Development", opt)
	}

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
	if len(bg2.Options) != len(bg.Options) {
		t.Errorf("options count: got %d, want %d", len(bg2.Options), len(bg.Options))
	}
	for i, opt := range bg.Options {
		if bg2.Options[i].Name != opt.Name || bg2.Options[i].DefaultValue != opt.DefaultValue {
			t.Errorf("option %d mismatch: got %+v, want %+v", i, bg2.Options[i], opt)
		}
	}
	if len(bg2.Properties) != len(bg.Properties) {
		t.Errorf("properties count: got %d, want %d", len(bg2.Properties), len(bg.Properties))
	}
	if bg2.Properties[0].Name != bg.Properties[0].Name || bg2.Properties[0].Value != bg.Properties[0].Value {
		t.Errorf("property mismatch: got %+v, want %+v", bg2.Properties[0], bg.Properties[0])
	}
	if len(bg2.Agents) != len(bg.Agents) {
		t.Errorf("agents count: got %d, want %d", len(bg2.Agents), len(bg.Agents))
	}
	for i, agent := range bg.Agents {
		if bg2.Agents[i].Name != agent.Name {
			t.Errorf("agent %d name mismatch: got %q, want %q", i, bg2.Agents[i].Name, agent.Name)
		}
		if len(bg2.Agents[i].Nodes) != len(agent.Nodes) {
			t.Errorf("agent %d nodes count: got %d, want %d", i, len(bg2.Agents[i].Nodes), len(agent.Nodes))
		}
	}
	if len(bg2.Aggregates) != len(bg.Aggregates) {
		t.Errorf("aggregates count: got %d, want %d", len(bg2.Aggregates), len(bg.Aggregates))
	}
	if bg2.Aggregates[0].Name != bg.Aggregates[0].Name || bg2.Aggregates[0].Requires != bg.Aggregates[0].Requires {
		t.Errorf("aggregate mismatch: got %+v, want %+v", bg2.Aggregates[0], bg.Aggregates[0])
	}
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
