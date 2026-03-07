package buildgraph

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/devrecon/ludus/internal/config"
)

// GenerateOption allows overriding defaults when generating a BuildGraph.
type GenerateOption func(*generateOptions)

type generateOptions struct {
	serverConfig string
}

// WithServerConfig overrides the server build configuration (e.g. "Shipping").
func WithServerConfig(c string) GenerateOption {
	return func(o *generateOptions) {
		o.serverConfig = c
	}
}

// Generate creates a BuildGraph XML structure from the given config and engine version.
// Options can override defaults (e.g. WithServerConfig("Shipping")).
func Generate(cfg *config.Config, engineVersion string, opts ...GenerateOption) (*BuildGraph, error) {
	if cfg.Engine.SourcePath == "" {
		return nil, fmt.Errorf("engine source path is required")
	}
	if cfg.Game.ProjectPath == "" {
		return nil, fmt.Errorf("game project path is required")
	}

	o := &generateOptions{
		serverConfig: "Development",
	}
	for _, fn := range opts {
		fn(o)
	}

	arch := cfg.Game.ResolvedArch()
	platform := config.UEPlatformName(arch)
	serverTarget := cfg.Game.ResolvedServerTarget()

	maxJobs := cfg.Engine.MaxJobs
	if maxJobs == 0 {
		maxJobs = 4
	}

	projectDir := filepath.Dir(cfg.Game.ProjectPath)
	serverArchiveDir := filepath.ToSlash(filepath.Join(projectDir, "PackagedServer"))
	clientArchiveDir := filepath.ToSlash(filepath.Join(projectDir, "PackagedClient"))

	bg := &BuildGraph{}

	// Options
	bg.Options = []Option{
		{Name: "SourcePath", DefaultValue: cfg.Engine.SourcePath, Description: "Path to UE5 engine source"},
		{Name: "ProjectPath", DefaultValue: cfg.Game.ProjectPath, Description: "Path to .uproject file"},
		{Name: "ProjectName", DefaultValue: cfg.Game.ProjectName, Description: "Project name"},
		{Name: "ServerTarget", DefaultValue: serverTarget, Description: "Server build target"},
		{Name: "Platform", DefaultValue: platform, Description: "Target platform"},
		{Name: "Arch", DefaultValue: arch, Description: "Target CPU architecture"},
		{Name: "MaxJobs", DefaultValue: strconv.Itoa(maxJobs), Description: "Max parallel compile jobs"},
		{Name: "ServerMap", DefaultValue: cfg.Game.ServerMap, Description: "Default server map"},
		{Name: "ServerConfig", DefaultValue: o.serverConfig, Description: "Build configuration (Development or Shipping)"},
	}

	// Properties
	bg.Properties = []Property{
		{Name: "EngineVersion", Value: engineVersion},
	}

	// Engine agent
	engineAgent := Agent{
		Name: "Engine",
		Nodes: []Node{
			{
				Name: "Setup",
				Steps: []Spawn{
					{
						Exe:        "bash",
						Arguments:  "Setup.sh",
						WorkingDir: "$(SourcePath)",
					},
				},
			},
			{
				Name:     "GenerateProjectFiles",
				Requires: "Setup",
				Steps: []Spawn{
					{
						Exe:        "bash",
						Arguments:  "GenerateProjectFiles.sh",
						WorkingDir: "$(SourcePath)",
					},
				},
			},
			{
				Name:     "CompileEngine",
				Requires: "GenerateProjectFiles",
				Steps: []Spawn{
					{
						Exe:        "make",
						Arguments:  "-j$(MaxJobs) ShaderCompileWorker",
						WorkingDir: "$(SourcePath)",
					},
					{
						Exe:        "make",
						Arguments:  "-j$(MaxJobs) UnrealEditor",
						WorkingDir: "$(SourcePath)",
					},
				},
			},
		},
	}
	bg.Agents = append(bg.Agents, engineAgent)

	// Game agent
	gameAgent := Agent{
		Name: "Game",
	}

	ruatPath := "Engine/Build/BatchFiles/RunUAT.sh"
	serverArgs := fmt.Sprintf(
		"BuildCookRun -project=$(ProjectPath) -noP4 -platform=%s -server -serverconfig=$(ServerConfig) -cook -build -stage -pak -archive -archivedirectory=%s",
		platform, serverArchiveDir,
	)
	if cfg.Game.ServerMap != "" {
		serverArgs += " -map=" + cfg.Game.ServerMap
	}

	buildServerNode := Node{
		Name: "BuildServer",
		Steps: []Spawn{
			{
				Exe:        ruatPath,
				Arguments:  serverArgs,
				WorkingDir: "$(SourcePath)",
			},
		},
	}
	gameAgent.Nodes = append(gameAgent.Nodes, buildServerNode)

	aggregateRequires := "BuildServer"

	if cfg.Game.ClientTarget != "" {
		clientArgs := fmt.Sprintf(
			"BuildCookRun -project=$(ProjectPath) -noP4 -platform=Win64 -client -cook -build -stage -pak -archive -archivedirectory=%s",
			clientArchiveDir,
		)

		buildClientNode := Node{
			Name: "BuildClient",
			Steps: []Spawn{
				{
					Exe:        ruatPath,
					Arguments:  clientArgs,
					WorkingDir: "$(SourcePath)",
				},
			},
		}
		gameAgent.Nodes = append(gameAgent.Nodes, buildClientNode)
		aggregateRequires += ";BuildClient"
	}

	bg.Agents = append(bg.Agents, gameAgent)

	// Aggregate
	bg.Aggregates = []Aggregate{
		{Name: "FullBuild", Requires: aggregateRequires},
	}

	return bg, nil
}
