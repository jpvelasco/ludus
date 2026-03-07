# BuildGraph XML Generation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `ludus buildgraph` command that generates UE5 BuildGraph XML from ludus.yaml config for consumption by Horde, UET, or any BuildGraph-compatible orchestrator.

**Architecture:** Go structs with `encoding/xml` tags model BuildGraph elements. A `Generate()` function populates the DAG from config. `xml.MarshalIndent()` produces the output. New `internal/buildgraph/` package, new `cmd/buildgraph/` command, new MCP tool.

**Tech Stack:** Go stdlib `encoding/xml`, Cobra command, existing `internal/config` and `internal/toolchain`.

---

### Task 1: Schema structs (`internal/buildgraph/schema.go`)

**Files:**
- Create: `internal/buildgraph/schema.go`

**Step 1: Create the schema file with all BuildGraph XML element structs**

```go
package buildgraph

import "encoding/xml"

// BuildGraph is the root element of a UE5 BuildGraph XML document.
type BuildGraph struct {
	XMLName    xml.Name    `xml:"BuildGraph"`
	Options    []Option    `xml:"Option"`
	Properties []Property  `xml:"Property"`
	Agents     []Agent     `xml:"Agent"`
	Aggregates []Aggregate `xml:"Aggregate"`
}

// Option is a parameterizable value that orchestrators can override at runtime.
type Option struct {
	Name         string `xml:"Name,attr"`
	DefaultValue string `xml:"DefaultValue,attr"`
	Description  string `xml:"Description,attr"`
}

// Property is a fixed value baked into the graph.
type Property struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// Agent groups nodes for execution on a specific machine type.
type Agent struct {
	Name  string `xml:"Name,attr"`
	Type  string `xml:"Type,attr,omitempty"`
	Nodes []Node `xml:"Node"`
}

// Node is an atomic build step with dependencies on other nodes.
type Node struct {
	Name     string  `xml:"Name,attr"`
	Requires string  `xml:"Requires,attr,omitempty"`
	Steps    []Spawn `xml:"Spawn"`
}

// Spawn executes a command within a node.
type Spawn struct {
	Exe        string `xml:"Exe,attr"`
	Arguments  string `xml:"Arguments,attr,omitempty"`
	WorkingDir string `xml:"WorkingDir,attr,omitempty"`
}

// Aggregate is a logical grouping that waits for dependencies without executing.
type Aggregate struct {
	Name     string `xml:"Name,attr"`
	Requires string `xml:"Requires,attr"`
}

// Marshal serializes the BuildGraph to indented XML with the XML declaration header.
func (bg *BuildGraph) Marshal() ([]byte, error) {
	output, err := xml.MarshalIndent(bg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), output...), nil
}
```

**Step 2: Verify it compiles**

Run: `cd "F:/Source Code/ludus" && go build ./internal/buildgraph/`
Expected: clean, no errors

**Step 3: Commit**

```bash
git add internal/buildgraph/schema.go
git commit -m "Add BuildGraph XML schema structs"
```

---

### Task 2: Generator with tests (`internal/buildgraph/generator.go`)

**Files:**
- Create: `internal/buildgraph/generator.go`
- Create: `internal/buildgraph/generator_test.go`

**Step 1: Write the failing tests**

Create `internal/buildgraph/generator_test.go`:

```go
package buildgraph

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func baseConfig() *config.Config {
	return &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "/opt/ue5",
			MaxJobs:    8,
		},
		Game: config.GameConfig{
			ProjectName: "Lyra",
			ProjectPath: "/opt/ue5/Samples/Games/Lyra/Lyra.uproject",
			ServerMap:   "L_Expanse",
		},
	}
}

func TestGenerate_AMD64(t *testing.T) {
	cfg := baseConfig()
	bg, err := Generate(cfg, "5.6.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check options exist
	optNames := make(map[string]string)
	for _, o := range bg.Options {
		optNames[o.Name] = o.DefaultValue
	}
	if optNames["SourcePath"] != "/opt/ue5" {
		t.Errorf("SourcePath option = %q, want /opt/ue5", optNames["SourcePath"])
	}
	if optNames["Platform"] != "Linux" {
		t.Errorf("Platform option = %q, want Linux", optNames["Platform"])
	}

	// Check engine agent nodes
	if len(bg.Agents) < 2 {
		t.Fatalf("expected at least 2 agents, got %d", len(bg.Agents))
	}
	engineAgent := bg.Agents[0]
	if engineAgent.Name != "Engine" {
		t.Errorf("first agent name = %q, want Engine", engineAgent.Name)
	}
	if len(engineAgent.Nodes) != 3 {
		t.Fatalf("engine agent nodes = %d, want 3", len(engineAgent.Nodes))
	}
	if engineAgent.Nodes[0].Name != "Setup" {
		t.Errorf("node 0 = %q, want Setup", engineAgent.Nodes[0].Name)
	}
	if engineAgent.Nodes[1].Requires != "Setup" {
		t.Errorf("GenerateProjectFiles requires = %q, want Setup", engineAgent.Nodes[1].Requires)
	}
	if engineAgent.Nodes[2].Requires != "GenerateProjectFiles" {
		t.Errorf("CompileEngine requires = %q, want GenerateProjectFiles", engineAgent.Nodes[2].Requires)
	}

	// Check game agent
	gameAgent := bg.Agents[1]
	if gameAgent.Name != "Game" {
		t.Errorf("second agent name = %q, want Game", gameAgent.Name)
	}
	if len(gameAgent.Nodes) != 1 {
		t.Fatalf("game agent nodes = %d, want 1 (server only)", len(gameAgent.Nodes))
	}
	if gameAgent.Nodes[0].Requires != "CompileEngine" {
		t.Errorf("BuildServer requires = %q, want CompileEngine", gameAgent.Nodes[0].Requires)
	}
}

func TestGenerate_ARM64(t *testing.T) {
	cfg := baseConfig()
	cfg.Game.Arch = "arm64"
	bg, err := Generate(cfg, "5.6.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	optNames := make(map[string]string)
	for _, o := range bg.Options {
		optNames[o.Name] = o.DefaultValue
	}
	if optNames["Platform"] != "LinuxArm64" {
		t.Errorf("Platform = %q, want LinuxArm64", optNames["Platform"])
	}
	if optNames["Arch"] != "arm64" {
		t.Errorf("Arch = %q, want arm64", optNames["Arch"])
	}
}

func TestGenerate_WithClient(t *testing.T) {
	cfg := baseConfig()
	cfg.Game.ClientTarget = "LyraGame"
	bg, err := Generate(cfg, "5.6.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gameAgent := bg.Agents[1]
	if len(gameAgent.Nodes) != 2 {
		t.Fatalf("game agent nodes = %d, want 2 (server + client)", len(gameAgent.Nodes))
	}
	if gameAgent.Nodes[1].Name != "BuildClient" {
		t.Errorf("node 1 = %q, want BuildClient", gameAgent.Nodes[1].Name)
	}

	// Aggregate should require both
	if len(bg.Aggregates) == 0 {
		t.Fatal("expected FullBuild aggregate")
	}
	requires := bg.Aggregates[0].Requires
	if !strings.Contains(requires, "BuildServer") || !strings.Contains(requires, "BuildClient") {
		t.Errorf("FullBuild requires = %q, want both BuildServer and BuildClient", requires)
	}
}

func TestGenerate_WithoutClient(t *testing.T) {
	cfg := baseConfig()
	bg, err := Generate(cfg, "5.6.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bg.Aggregates) == 0 {
		t.Fatal("expected FullBuild aggregate")
	}
	if bg.Aggregates[0].Requires != "BuildServer" {
		t.Errorf("FullBuild requires = %q, want BuildServer only", bg.Aggregates[0].Requires)
	}
}

func TestGenerate_Shipping(t *testing.T) {
	cfg := baseConfig()
	cfg.Game.ServerConfig = "Shipping"
	bg, err := Generate(cfg, "5.6.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	optNames := make(map[string]string)
	for _, o := range bg.Options {
		optNames[o.Name] = o.DefaultValue
	}
	if optNames["ServerConfig"] != "Shipping" {
		t.Errorf("ServerConfig = %q, want Shipping", optNames["ServerConfig"])
	}
}

func TestGenerate_XMLRoundTrip(t *testing.T) {
	cfg := baseConfig()
	bg, err := Generate(cfg, "5.6.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := bg.Marshal()
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Verify it starts with XML declaration
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Error("output should start with XML declaration")
	}

	// Round-trip: unmarshal back
	var bg2 BuildGraph
	if err := xml.Unmarshal(data, &bg2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(bg2.Agents) != len(bg.Agents) {
		t.Errorf("round-trip agents: got %d, want %d", len(bg2.Agents), len(bg.Agents))
	}
	if len(bg2.Options) != len(bg.Options) {
		t.Errorf("round-trip options: got %d, want %d", len(bg2.Options), len(bg.Options))
	}
}

func TestGenerate_MissingSourcePath(t *testing.T) {
	cfg := baseConfig()
	cfg.Engine.SourcePath = ""
	_, err := Generate(cfg, "5.6.1")
	if err == nil {
		t.Fatal("expected error for missing source path")
	}
}

func TestGenerate_MissingProjectPath(t *testing.T) {
	cfg := baseConfig()
	cfg.Game.ProjectPath = ""
	_, err := Generate(cfg, "5.6.1")
	if err == nil {
		t.Fatal("expected error for missing project path")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd "F:/Source Code/ludus" && go test ./internal/buildgraph/ -v`
Expected: FAIL — `Generate` not defined

**Step 3: Write the generator implementation**

Create `internal/buildgraph/generator.go`:

```go
package buildgraph

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/devrecon/ludus/internal/config"
)

// Generate creates a BuildGraph XML struct from ludus config and engine version.
// The generated graph covers engine build (Setup, GenerateProjectFiles, Compile)
// and game build (BuildServer, optionally BuildClient) stages.
func Generate(cfg *config.Config, engineVersion string) (*BuildGraph, error) {
	if cfg.Engine.SourcePath == "" {
		return nil, fmt.Errorf("engine source path required for BuildGraph generation (set engine.sourcePath in ludus.yaml)")
	}
	if cfg.Game.ProjectPath == "" {
		return nil, fmt.Errorf("game project path required for BuildGraph generation (set game.projectPath in ludus.yaml)")
	}

	arch := cfg.Game.ResolvedArch()
	platform := config.UEPlatformName(arch)
	serverTarget := cfg.Game.ResolvedServerTarget()

	serverConfig := cfg.Game.ServerConfig
	if serverConfig == "" {
		serverConfig = "Development"
	}

	maxJobs := cfg.Engine.MaxJobs
	if maxJobs == 0 {
		maxJobs = 4
	}

	bg := &BuildGraph{}

	// Options — overridable by orchestrators at runtime
	bg.Options = []Option{
		{Name: "SourcePath", DefaultValue: cfg.Engine.SourcePath, Description: "Path to UE5 engine source"},
		{Name: "ProjectPath", DefaultValue: cfg.Game.ProjectPath, Description: "Path to .uproject file"},
		{Name: "ProjectName", DefaultValue: cfg.Game.ProjectName, Description: "UE5 project name"},
		{Name: "ServerTarget", DefaultValue: serverTarget, Description: "Dedicated server build target"},
		{Name: "Platform", DefaultValue: platform, Description: "Target platform (Linux or LinuxArm64)"},
		{Name: "Arch", DefaultValue: arch, Description: "Target CPU architecture (amd64 or arm64)"},
		{Name: "MaxJobs", DefaultValue: strconv.Itoa(maxJobs), Description: "Max parallel compile actions"},
		{Name: "ServerMap", DefaultValue: cfg.Game.ServerMap, Description: "Default server map"},
		{Name: "ServerConfig", DefaultValue: serverConfig, Description: "Build configuration (Development or Shipping)"},
	}

	// Engine version as a fixed property
	bg.Properties = []Property{
		{Name: "EngineVersion", Value: engineVersion},
	}

	// Engine agent: Setup → GenerateProjectFiles → CompileEngine
	runatPath := filepath.Join("Engine", "Build", "BatchFiles", "RunUAT.sh")
	bg.Agents = append(bg.Agents, Agent{
		Name: "Engine",
		Type: "CompileLinux",
		Nodes: []Node{
			{
				Name: "Setup",
				Steps: []Spawn{{
					Exe:        "bash",
					Arguments:  "Setup.sh",
					WorkingDir: "$(SourcePath)",
				}},
			},
			{
				Name:     "GenerateProjectFiles",
				Requires: "Setup",
				Steps: []Spawn{{
					Exe:        "bash",
					Arguments:  "GenerateProjectFiles.sh",
					WorkingDir: "$(SourcePath)",
				}},
			},
			{
				Name:     "CompileEngine",
				Requires: "GenerateProjectFiles",
				Steps: []Spawn{
					{
						Exe:        "make",
						Arguments:  fmt.Sprintf("-j$(MaxJobs) ShaderCompileWorker"),
						WorkingDir: "$(SourcePath)",
					},
					{
						Exe:        "make",
						Arguments:  fmt.Sprintf("-j$(MaxJobs) UnrealEditor"),
						WorkingDir: "$(SourcePath)",
					},
				},
			},
		},
	})

	// Game agent: BuildServer (and optionally BuildClient)
	serverArgs := fmt.Sprintf(
		`BuildCookRun -project="$(ProjectPath)" -platform=$(Platform) -server -noclient -servertargetname=$(ServerTarget) -serverconfig=$(ServerConfig) -build -cook -stage -package -archive -archivedirectory="%s"`,
		filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer"),
	)
	if cfg.Game.ServerMap != "" {
		serverArgs += " -map=" + cfg.Game.ServerMap
	}

	gameNodes := []Node{
		{
			Name:     "BuildServer",
			Requires: "CompileEngine",
			Steps: []Spawn{{
				Exe:        "bash",
				Arguments:  runatPath + " " + serverArgs,
				WorkingDir: "$(SourcePath)",
			}},
		},
	}

	// Optional client build
	if cfg.Game.ClientTarget != "" {
		clientArgs := fmt.Sprintf(
			`BuildCookRun -project="$(ProjectPath)" -platform=Linux -client -build -cook -stage -package -archive -archivedirectory="%s"`,
			filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedClient"),
		)
		gameNodes = append(gameNodes, Node{
			Name:     "BuildClient",
			Requires: "CompileEngine",
			Steps: []Spawn{{
				Exe:        "bash",
				Arguments:  runatPath + " " + clientArgs,
				WorkingDir: "$(SourcePath)",
			}},
		})
	}

	bg.Agents = append(bg.Agents, Agent{
		Name:  "Game",
		Type:  "CompileLinux",
		Nodes: gameNodes,
	})

	// Aggregate: FullBuild
	aggregateRequires := "BuildServer"
	if cfg.Game.ClientTarget != "" {
		aggregateRequires += ";BuildClient"
	}
	bg.Aggregates = []Aggregate{
		{Name: "FullBuild", Requires: aggregateRequires},
	}

	return bg, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd "F:/Source Code/ludus" && go test ./internal/buildgraph/ -v`
Expected: all 8 tests PASS

**Step 5: Run lint**

Run: `cd "F:/Source Code/ludus" && golangci-lint run ./internal/buildgraph/`
Expected: 0 issues

**Step 6: Commit**

```bash
git add internal/buildgraph/generator.go internal/buildgraph/generator_test.go
git commit -m "Add BuildGraph generator with tests"
```

---

### Task 3: CLI command (`cmd/buildgraph/buildgraph.go`)

**Files:**
- Create: `cmd/buildgraph/buildgraph.go`
- Modify: `cmd/root/root.go` — add import and `rootCmd.AddCommand()`

**Step 1: Create the command file**

```go
package buildgraph

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/buildgraph"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	outputPath string
	toStdout   bool
)

// Cmd is the buildgraph command.
var Cmd = &cobra.Command{
	Use:   "buildgraph",
	Short: "Generate UE5 BuildGraph XML from ludus config",
	Long: `Generates a BuildGraph XML file describing the engine and game build pipeline
as a directed acyclic graph (DAG). The output is consumable by Horde, UET, or
any BuildGraph-compatible orchestrator.

By default writes to Build/BuildGraph.xml in the project directory.
Use --output/-o to specify a custom path, or --stdout to print to stdout.`,
	RunE: runBuildGraph,
}

func init() {
	Cmd.Flags().StringVarP(&outputPath, "output", "o", "", "output file path (default: Build/BuildGraph.xml in project dir)")
	Cmd.Flags().BoolVar(&toStdout, "stdout", false, "print XML to stdout instead of writing to file")
}

func runBuildGraph(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	bg, err := buildgraph.Generate(cfg, engineVersion)
	if err != nil {
		return err
	}

	data, err := bg.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling BuildGraph XML: %w", err)
	}

	if toStdout {
		_, err = os.Stdout.Write(data)
		return err
	}

	// Resolve output path
	outPath := outputPath
	if outPath == "" {
		projectDir := filepath.Dir(cfg.Game.ProjectPath)
		if projectDir == "." || projectDir == "" {
			projectDir = "."
		}
		outPath = filepath.Join(projectDir, "Build", "BuildGraph.xml")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	fmt.Printf("BuildGraph XML written to %s\n", outPath)
	return nil
}
```

**Step 2: Register command in `cmd/root/root.go`**

Add import:
```go
"github.com/devrecon/ludus/cmd/buildgraph"
```

Add to `init()` after the `ci.Cmd` line:
```go
rootCmd.AddCommand(buildgraph.Cmd)
```

**Step 3: Verify it compiles**

Run: `cd "F:/Source Code/ludus" && go build -o /dev/null .`
Expected: clean

**Step 4: Test the command with dry config**

Run: `cd "F:/Source Code/ludus" && go run . buildgraph --stdout`
Expected: XML output to terminal (or an error about missing config, which is fine — confirms the command is wired up)

**Step 5: Commit**

```bash
git add cmd/buildgraph/buildgraph.go cmd/root/root.go
git commit -m "Add ludus buildgraph CLI command"
```

---

### Task 4: MCP tool (`cmd/mcp/tools_buildgraph.go`)

**Files:**
- Create: `cmd/mcp/tools_buildgraph.go`
- Modify: `cmd/mcp/register.go` — add `registerBuildGraphTool(s)` call

**Step 1: Create the MCP tool file**

```go
package mcp

import (
	"context"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/buildgraph"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type buildGraphInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"Print commands without executing"`
}

func registerBuildGraphTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ludus_buildgraph",
		Description: "Generate UE5 BuildGraph XML from ludus config. Returns the XML content describing engine and game build stages as a DAG for Horde, UET, or other orchestrators.",
	}, handleBuildGraph)
}

func handleBuildGraph(_ context.Context, _ *mcp.CallToolRequest, _ buildGraphInput) (*mcp.CallToolResult, any, error) {
	cfg := globals.Cfg

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	bg, err := buildgraph.Generate(cfg, engineVersion)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(map[string]string{"error": err.Error()})},
			},
		}, nil, nil
	}

	data, err := bg.Marshal()
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: jsonString(map[string]string{"error": err.Error()})},
			},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}
```

**Step 2: Register in `cmd/mcp/register.go`**

Add after `registerAsyncBuildTools(s)`:
```go
registerBuildGraphTool(s)
```

**Step 3: Verify it compiles**

Run: `cd "F:/Source Code/ludus" && go build -o /dev/null .`
Expected: clean

**Step 4: Run full test suite**

Run: `cd "F:/Source Code/ludus" && go test ./...`
Expected: all pass

**Step 5: Run lint**

Run: `cd "F:/Source Code/ludus" && golangci-lint run ./...`
Expected: 0 issues

**Step 6: Commit**

```bash
git add cmd/mcp/tools_buildgraph.go cmd/mcp/register.go
git commit -m "Add ludus_buildgraph MCP tool"
```

---

### Task 5: Update CLAUDE.md and ROADMAP.md

**Files:**
- Modify: `CLAUDE.md` — add `ludus buildgraph` to command list, add `buildgraph` to internal packages table, increment MCP tool count
- Modify: `ROADMAP.md` — check off BuildGraph item

**Step 1: Update CLAUDE.md command list**

Add after the `ludus ci` line:
```
ludus buildgraph                   # generate BuildGraph XML for Horde/UET
```

Add to internal packages table:
```
| `buildgraph` | BuildGraph XML generation: schema structs, DAG generator from config |
```

Update MCP tool count from "20 tools" to "21 tools".

**Step 2: Update ROADMAP.md**

Change:
```
- [ ] **BuildGraph XML generation** — ...
```
To:
```
- [x] **BuildGraph XML generation** — ...
```

**Step 3: Commit**

```bash
git add CLAUDE.md ROADMAP.md
git commit -m "Document BuildGraph command in CLAUDE.md and ROADMAP.md"
```

---

### Task 6: Final validation and PR

**Step 1: Run full validation**

```bash
cd "F:/Source Code/ludus"
go build -o /dev/null .
go vet ./...
golangci-lint run ./...
go test ./...
```
Expected: all clean

**Step 2: Test the actual command output**

```bash
go run . buildgraph --stdout
```
Expected: well-formed BuildGraph XML with Options, Engine agent (3 nodes), Game agent (1+ nodes), FullBuild aggregate

**Step 3: Push and create PR**

```bash
git push -u origin feat/buildgraph-xml
gh pr create --title "Add BuildGraph XML generation command" --body "..."
```
