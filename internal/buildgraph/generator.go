package buildgraph

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/jpvelasco/ludus/internal/config"
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
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	o := newGenerateOptions(opts...)
	input := newGraphInput(cfg, engineVersion, o)

	bg := &BuildGraph{}
	bg.Options = buildOptions(input)
	bg.Properties = []Property{{Name: "EngineVersion", Value: engineVersion}}
	gameAgent, aggregateRequires := buildGameAgent(input)
	bg.Agents = append(bg.Agents, buildEngineAgent(), gameAgent)
	bg.Aggregates = []Aggregate{{Name: "FullBuild", Requires: aggregateRequires}}

	return bg, nil
}

type graphInput struct {
	cfg               *config.Config
	engineVersion     string
	serverConfig      string
	arch              string
	platform          string
	serverTarget      string
	maxJobs           int
	serverArchiveDir  string
	clientArchiveDir  string
	aggregateRequires string
}

func validateConfig(cfg *config.Config) error {
	if cfg.Engine.SourcePath == "" {
		return fmt.Errorf("engine source path is required")
	}
	if cfg.Game.ProjectPath == "" {
		return fmt.Errorf("game project path is required")
	}
	return nil
}

func newGenerateOptions(opts ...GenerateOption) *generateOptions {
	o := &generateOptions{serverConfig: "Development"}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

func newGraphInput(cfg *config.Config, engineVersion string, opts *generateOptions) graphInput {
	arch := cfg.Game.ResolvedArch()
	projectDir := filepath.Dir(cfg.Game.ProjectPath)
	input := graphInput{
		cfg:              cfg,
		engineVersion:    engineVersion,
		serverConfig:     opts.serverConfig,
		arch:             arch,
		platform:         config.UEPlatformName(arch),
		serverTarget:     cfg.Game.ResolvedServerTarget(),
		maxJobs:          defaultMaxJobs(cfg.Engine.MaxJobs),
		serverArchiveDir: filepath.ToSlash(filepath.Join(projectDir, "PackagedServer")),
		clientArchiveDir: filepath.ToSlash(filepath.Join(projectDir, "PackagedClient")),
	}
	input.aggregateRequires = "BuildServer"
	return input
}

func defaultMaxJobs(maxJobs int) int {
	if maxJobs == 0 {
		return 4
	}
	return maxJobs
}

func buildOptions(input graphInput) []Option {
	cfg := input.cfg
	return []Option{
		{Name: "SourcePath", DefaultValue: cfg.Engine.SourcePath, Description: "Path to UE5 engine source"},
		{Name: "ProjectPath", DefaultValue: cfg.Game.ProjectPath, Description: "Path to .uproject file"},
		{Name: "ProjectName", DefaultValue: cfg.Game.ProjectName, Description: "Project name"},
		{Name: "ServerTarget", DefaultValue: input.serverTarget, Description: "Server build target"},
		{Name: "Platform", DefaultValue: input.platform, Description: "Target platform"},
		{Name: "Arch", DefaultValue: input.arch, Description: "Target CPU architecture"},
		{Name: "MaxJobs", DefaultValue: strconv.Itoa(input.maxJobs), Description: "Max parallel compile jobs"},
		{Name: "ServerMap", DefaultValue: cfg.Game.ServerMap, Description: "Default server map"},
		{Name: "ServerConfig", DefaultValue: input.serverConfig, Description: "Build configuration (Development or Shipping)"},
	}
}

func buildEngineAgent() Agent {
	return Agent{
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
}

func buildGameAgent(input graphInput) (Agent, string) {
	gameAgent := Agent{
		Name: "Game",
	}
	aggregateRequires := input.aggregateRequires

	ruatPath := "Engine/Build/BatchFiles/RunUAT.sh"
	serverArgs := fmt.Sprintf(
		"BuildCookRun -project=$(ProjectPath) -noP4 -platform=%s -server -serverconfig=$(ServerConfig) -cook -build -stage -pak -archive -archivedirectory=%s",
		input.platform, input.serverArchiveDir,
	)
	if input.cfg.Game.ServerMap != "" {
		serverArgs += " -map=" + input.cfg.Game.ServerMap
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

	if input.cfg.Game.ClientTarget != "" {
		clientArgs := fmt.Sprintf(
			"BuildCookRun -project=$(ProjectPath) -noP4 -platform=Win64 -client -cook -build -stage -pak -archive -archivedirectory=%s",
			input.clientArchiveDir,
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

	return gameAgent, aggregateRequires
}
