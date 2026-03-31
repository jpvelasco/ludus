package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/dockerbuild"
	gameBuilder "github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

// saveClientState persists client build info to state.
func saveClientState(result *gameBuilder.ClientBuildResult) {
	if err := state.UpdateClient(&state.ClientState{
		BinaryPath: result.ClientBinary,
		OutputDir:  result.OutputDir,
		Platform:   result.Platform,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}
}

// printBuildConfigGuidance prints a note about the build configuration.
func printBuildConfigGuidance(cfg string) {
	switch strings.ToLower(cfg) {
	case "shipping":
		fmt.Println("Build config: Shipping (optimized, smaller binaries, no debug symbols)")
	case "development", "":
		// Only print if the user explicitly chose Development or didn't set it
		if cfg != "" {
			fmt.Println("Build config: Development (debug symbols, larger binaries, faster iteration)")
		}
	default:
		fmt.Printf("Build config: %s\n", cfg)
	}
}

// nextAfterServerBuild returns the "Next:" hint based on the deploy target.
func nextAfterServerBuild() string {
	t := strings.ToLower(globals.Cfg.Deploy.Target)
	switch t {
	case "gamelift", "stack":
		return "ludus container build"
	case "ec2", "anywhere", "binary":
		return fmt.Sprintf("ludus deploy %s", t)
	default:
		return "ludus container build  (or: ludus deploy <target>)"
	}
}

var (
	skipCook       bool
	skipCookClient bool
	clientPlatform string
	backend        string
	noCache        bool
	noCacheClient  bool
	serverConfig   string
	maxJobs        int
	maxJobsClient  int
	archFlag       string
)

// Cmd is the top-level game command group.
var Cmd = &cobra.Command{
	Use:   "game",
	Short: "Build and configure the UE5 game dedicated server",
	Long: `Commands for building a UE5 project as a dedicated server.
This handles compiling the server target, cooking content for Linux,
and packaging the build for containerization.`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the game as a Linux dedicated server",
	Long: `Builds the UE5 project using RunUAT BuildCookRun:

  1. Build the server target for Linux
  2. Cook content for the Linux server platform
  3. Stage and package the server build
  4. Output a ready-to-containerize server directory

Use --backend docker to build inside a pre-built engine Docker image.

Build configurations (--build-config):
  Development  Faster builds, includes debug symbols, larger binaries (~2-3 GB).
               Good for iteration and debugging. Default if --build-config is not specified.
  Shipping     Optimized for production: smaller binaries (~1-1.5 GB), no debug
               symbols, no console commands, stripped logging. Use for final deployment.`,
	RunE: runBuild,
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Build the game as a standalone game client",
	Long: `Builds the UE5 project using RunUAT BuildCookRun as a game client:

  1. Build the game client target for the specified platform
  2. Cook content for the client platform
  3. Stage and package the client build
  4. Output a ready-to-run client directory

Use --platform to target a different platform (default: Linux).
Win64 cross-compilation requires the Windows cross-compile toolchain.`,
	RunE: runClientBuild,
}

func init() {
	buildCmd.Flags().BoolVar(&skipCook, "skip-cook", false, "skip content cooking (use previously cooked content)")
	buildCmd.Flags().StringVar(&backend, "backend", "", `build backend: "native" or "docker" (default: from ludus.yaml)`)
	buildCmd.Flags().BoolVar(&noCache, "no-cache", false, "disable build caching (forces rebuild even if inputs are unchanged)")
	buildCmd.Flags().StringVar(&serverConfig, "build-config", "", `build configuration: "Development" or "Shipping" (default: Development)`)
	buildCmd.Flags().IntVarP(&maxJobs, "jobs", "j", 0, "max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)")
	buildCmd.Flags().StringVar(&archFlag, "arch", "", `target CPU architecture: amd64, arm64 (default: from ludus.yaml)`)
	clientCmd.Flags().BoolVar(&skipCookClient, "skip-cook", false, "skip content cooking (use previously cooked content)")
	clientCmd.Flags().StringVar(&backend, "backend", "", `build backend: "native" or "docker" (default: from ludus.yaml)`)
	clientCmd.Flags().BoolVar(&noCacheClient, "no-cache", false, "disable build caching (forces rebuild even if inputs are unchanged)")
	clientCmd.Flags().StringVar(&clientPlatform, "platform", "Linux", "target platform (Linux, Win64)")
	clientCmd.Flags().IntVarP(&maxJobsClient, "jobs", "j", 0, "max parallel compile actions (0 = auto-detect from RAM)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(clientCmd)
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string {
	if backend != "" {
		return backend
	}
	return globals.Cfg.Engine.Backend
}

// resolveArch returns the effective architecture, preferring CLI flag over config.
func resolveArch() string {
	if archFlag != "" {
		return config.NormalizeArch(archFlag)
	}
	return globals.Cfg.Game.ResolvedArch()
}

// resolveEngineImage determines the Docker image to use for game builds.
// Precedence: config DockerImage > state EngineImage > constructed from config.
func resolveEngineImage() (string, error) {
	cfg := globals.Cfg

	// Explicit pre-built image from config
	if cfg.Engine.DockerImage != "" {
		return cfg.Engine.DockerImage, nil
	}

	// Check state for recently built image
	s, err := state.Load()
	if err == nil && s.EngineImage != nil && s.EngineImage.ImageTag != "" {
		return s.EngineImage.ImageTag, nil
	}

	// Construct from config defaults
	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}
	version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	tag := version
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s:%s", imageName, tag), nil
}

func runBuild(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckGameReady()); err != nil {
		return err
	}

	if resolveBackend() == "docker" {
		return runDockerBuild(cmd)
	}

	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)

	if cache.CheckSkip(cache.StageGameServer, serverHash, cfg.Game.ProjectName, noCache) {
		return nil
	}

	enginePath := cfg.Engine.SourcePath
	if enginePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml)")
	}

	engineVersion, _ := toolchain.DetectEngineVersion(enginePath, cfg.Engine.Version)

	arch := resolveArch()
	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
		EnginePath:    enginePath,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		Platform:      cfg.Game.Platform,
		Arch:          arch,
		ServerOnly:    true,
		SkipCook:      skipCook,
		ServerMap:     cfg.Game.ServerMap,
		EngineVersion: engineVersion,
		ServerConfig:  serverConfig,
		MaxJobs:       maxJobs,
	}, r)

	if hint := builder.PartialBuildHint(); hint != "" {
		fmt.Printf("Tip: %s\n", hint)
	}

	printBuildConfigGuidance(serverConfig)
	fmt.Printf("Building %s dedicated server (%s)...\n", cfg.Game.ProjectName, arch)
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	cache.RecordBuild(cache.StageGameServer, serverHash)

	fmt.Printf("%s server build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("\nNext: %s\n", nextAfterServerBuild())
	return nil
}

func runDockerBuild(cmd *cobra.Command) error {
	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)

	if cache.CheckSkip(cache.StageGameServer, serverHash, cfg.Game.ProjectName, noCache) {
		return nil
	}

	engineImage, err := resolveEngineImage()
	if err != nil {
		return err
	}

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		SkipCook:      skipCook,
		ServerMap:     cfg.Game.ServerMap,
		EngineVersion: engineVersion,
	}, r)

	fmt.Printf("Building %s dedicated server in Docker (image: %s)...\n", cfg.Game.ProjectName, engineImage)
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	cache.RecordBuild(cache.StageGameServer, serverHash)

	fmt.Printf("%s server build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("\nNext: %s\n", nextAfterServerBuild())
	return nil
}

func runClientBuild(cmd *cobra.Command, args []string) error {
	if resolveBackend() == "docker" {
		return runDockerClientBuild(cmd)
	}

	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, clientPlatform)

	if cache.CheckSkip(cache.StageGameClient, clientHash, cfg.Game.ProjectName, noCacheClient) {
		return nil
	}

	enginePath := cfg.Engine.SourcePath
	if enginePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml)")
	}

	engineVersion, _ := toolchain.DetectEngineVersion(enginePath, cfg.Engine.Version)

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
		EnginePath:     enginePath,
		ProjectPath:    cfg.Game.ProjectPath,
		ProjectName:    cfg.Game.ProjectName,
		ClientTarget:   cfg.Game.ResolvedClientTarget(),
		ClientPlatform: clientPlatform,
		SkipCook:       skipCookClient,
		EngineVersion:  engineVersion,
		MaxJobs:        maxJobsClient,
	}, r)

	if hint := builder.PartialClientBuildHint(); hint != "" {
		fmt.Printf("Tip: %s\n", hint)
	}

	fmt.Printf("Building %s standalone client for %s...\n", cfg.Game.ProjectName, clientPlatform)
	result, err := builder.BuildClient(cmd.Context())
	if err != nil {
		return err
	}

	saveClientState(result)
	cache.RecordBuild(cache.StageGameClient, clientHash)

	fmt.Printf("%s client build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("Binary: %s\n", result.ClientBinary)
	fmt.Println("\nNext: ludus connect")
	return nil
}

func runDockerClientBuild(cmd *cobra.Command) error {
	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, clientPlatform)

	if cache.CheckSkip(cache.StageGameClient, clientHash, cfg.Game.ProjectName, noCacheClient) {
		return nil
	}

	engineImage, err := resolveEngineImage()
	if err != nil {
		return err
	}

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:    engineImage,
		ProjectPath:    cfg.Game.ProjectPath,
		ProjectName:    cfg.Game.ProjectName,
		ClientTarget:   cfg.Game.ResolvedClientTarget(),
		ClientPlatform: clientPlatform,
		SkipCook:       skipCookClient,
		EngineVersion:  engineVersion,
	}, r)

	fmt.Printf("Building %s standalone client in Docker for %s (image: %s)...\n",
		cfg.Game.ProjectName, clientPlatform, engineImage)
	result, err := builder.BuildClient(cmd.Context())
	if err != nil {
		return err
	}

	saveClientState(result)
	cache.RecordBuild(cache.StageGameClient, clientHash)

	fmt.Printf("%s client build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("Binary: %s\n", result.ClientBinary)
	fmt.Println("\nNext: ludus connect")
	return nil
}
