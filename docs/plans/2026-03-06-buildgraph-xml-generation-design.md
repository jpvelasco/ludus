# BuildGraph XML Generation — Design

## Problem

Ludus has a linear, sequential pipeline that works well for single-machine builds. Studios using distributed build systems (Horde, UET, or custom orchestrators) need a way to express the same build steps as a BuildGraph DAG so their tools can manage parallelism, machine assignment, and artifact flow.

## Decision

Add a `ludus buildgraph` command that generates UE5 BuildGraph XML from `ludus.yaml` config. The XML is generic and schema-valid — any BuildGraph-compatible consumer can use it. Ludus does not execute the XML; it only generates it.

## Scope

Build stages only:

- **Engine**: Setup, GenerateProjectFiles, CompileEngine
- **Game**: BuildCookRunServer, BuildClient (optional)

Container and deploy stages are excluded — they involve Docker and AWS SDK calls, not UE build commands.

## Architecture

Hybrid approach: Go structs with `encoding/xml` tags model BuildGraph elements. A programmatic builder populates the struct tree from config. `xml.MarshalIndent()` produces the output.

### Package: `internal/buildgraph/`

**`schema.go`** — BuildGraph XML element structs:

```go
type BuildGraph struct {
    XMLName    xml.Name    `xml:"BuildGraph"`
    Options    []Option    `xml:"Option"`
    Properties []Property  `xml:"Property"`
    Agents     []Agent     `xml:"Agent"`
    Aggregates []Aggregate `xml:"Aggregate"`
}

type Option struct {
    Name         string `xml:"Name,attr"`
    DefaultValue string `xml:"DefaultValue,attr"`
    Description  string `xml:"Description,attr"`
}

type Property struct {
    Name  string `xml:"Name,attr"`
    Value string `xml:"Value,attr"`
}

type Agent struct {
    Name  string `xml:"Name,attr"`
    Type  string `xml:"Type,attr,omitempty"`
    Nodes []Node `xml:"Node"`
}

type Node struct {
    Name     string  `xml:"Name,attr"`
    Requires string  `xml:"Requires,attr,omitempty"`
    Steps    []Spawn `xml:"Spawn"`
}

type Spawn struct {
    Exe        string `xml:"Exe,attr"`
    Arguments  string `xml:"Arguments,attr,omitempty"`
    WorkingDir string `xml:"WorkingDir,attr,omitempty"`
}

type Aggregate struct {
    Name     string `xml:"Name,attr"`
    Requires string `xml:"Requires,attr"`
}
```

**`generator.go`** — `Generate(cfg *config.Config, engineVersion string) (*BuildGraph, error)`

Reads config and engine version to populate the BuildGraph struct:

1. Creates `<Option>` elements with defaults from config (SourcePath, ProjectPath, ProjectName, ServerTarget, Platform, Arch, MaxJobs, ServerMap, ServerConfig)
2. Creates "Engine" agent with Setup, GenerateProjectFiles, CompileEngine nodes
3. Creates "Game" agent with BuildServer node (and BuildClient if client target is configured)
4. Creates "FullBuild" aggregate requiring all terminal nodes
5. Returns the populated struct

**Parameterization**: Config values become `<Option DefaultValue="...">` elements. Orchestrators override at runtime via `--set:OptionName=value`.

### Command: `cmd/buildgraph/buildgraph.go`

```
ludus buildgraph              # writes to Build/BuildGraph.xml in project dir
ludus buildgraph -o out.xml   # custom output path
ludus buildgraph --stdout     # print to stdout
```

Registered in `cmd/root/root.go` like all other subcommands.

### MCP: `cmd/mcp/tools_buildgraph.go`

`ludus_buildgraph` tool — calls `buildgraph.Generate()`, returns XML as text content. No file I/O.

### Node Graph

```
Options: SourcePath, ProjectPath, ProjectName, ServerTarget,
         Platform, Arch, MaxJobs, ServerMap, ServerConfig

Agent "Engine":
  Node "Setup"                    Requires=""
  Node "GenerateProjectFiles"     Requires="Setup"
  Node "CompileEngine"            Requires="GenerateProjectFiles"

Agent "Game":
  Node "BuildServer"              Requires="CompileEngine"
  Node "BuildClient" (optional)   Requires="CompileEngine"

Aggregate "FullBuild"             Requires="BuildServer[;BuildClient]"
```

### Data Flow

```
ludus.yaml → config.Load() → buildgraph.Generate(cfg, engineVersion)
           → *BuildGraph struct → xml.MarshalIndent() → file or stdout
```

## Error Handling

- Missing `engine.sourcePath` → error with guidance
- Missing `game.projectPath` → error with guidance
- File write failure → standard Go error with path
- Invalid arch → caught by `NormalizeArch()` upstream

No network calls, no AWS, no Docker. Pure local XML generation.

## Testing

Unit tests in `internal/buildgraph/`:

- `TestGenerate_AMD64` — default config, verify node names/dependencies/commands
- `TestGenerate_ARM64` — arch changes platform to LinuxArm64
- `TestGenerate_WithClient` — client node present when client target set
- `TestGenerate_WithoutClient` — no client node when not configured
- `TestGenerate_Shipping` — ServerConfig option defaults to Shipping
- `TestGenerate_XMLRoundTrip` — marshal then unmarshal, verify no data loss
- `TestGenerate_MissingSourcePath` — error case
- `TestGenerate_MissingProjectPath` — error case

## No New Dependencies

Uses only `encoding/xml` from stdlib. No new config fields. No changes to existing packages.
