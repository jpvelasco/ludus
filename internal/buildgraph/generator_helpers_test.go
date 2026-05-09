package buildgraph

import (
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
