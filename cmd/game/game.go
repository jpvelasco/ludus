package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	gameBuilder "github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/prereq"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/toolchain"
	"github.com/jpvelasco/ludus/internal/wsl"
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

Use --backend podman or --backend docker to build inside a pre-built engine container image.

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
	buildCmd.Flags().StringVar(&backend, "backend", "", `build backend: "native", "podman" (recommended), or "docker" (default: from ludus.yaml)`)
	buildCmd.Flags().BoolVar(&noCache, "no-cache", false, "disable build caching (forces rebuild even if inputs are unchanged)")
	buildCmd.Flags().StringVar(&serverConfig, "build-config", "", `build configuration: "Development" or "Shipping" (default: Development)`)
	buildCmd.Flags().IntVarP(&maxJobs, "jobs", "j", 0, "max parallel compile actions (0 = auto-detect from RAM, halved for cross-compile)")
	buildCmd.Flags().StringVar(&archFlag, "arch", "", `target CPU architecture: amd64, arm64 (default: from ludus.yaml)`)
	clientCmd.Flags().BoolVar(&skipCookClient, "skip-cook", false, "skip content cooking (use previously cooked content)")
	clientCmd.Flags().StringVar(&backend, "backend", "", `build backend: "native", "podman" (recommended), or "docker" (default: from ludus.yaml)`)
	clientCmd.Flags().BoolVar(&noCacheClient, "no-cache", false, "disable build caching (forces rebuild even if inputs are unchanged)")
	clientCmd.Flags().StringVar(&clientPlatform, "platform", "Linux", "target platform (Linux, Win64)")
	clientCmd.Flags().IntVarP(&maxJobsClient, "jobs", "j", 0, "max parallel compile actions (0 = auto-detect from RAM)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(clientCmd)
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string { return globals.ResolveBackend(backend) }

// resolveArch returns the effective architecture, preferring CLI flag over config.
func resolveArch() string {
	if archFlag != "" {
		return config.NormalizeArch(archFlag)
	}
	return globals.Cfg.Game.ResolvedArch()
}

func resolvedBuildConfig() config.Config {
	cfg := globals.Cfg.Clone()
	cfg.Game.Arch = resolveArch()
	return cfg
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg := resolvedBuildConfig()
	checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
	if err := prereq.Validate(checker.CheckGameReady()); err != nil {
		return err
	}

	be := resolveBackend()
	if dockerbuild.IsWSL2Backend(be) {
		return runWSL2GameBuild(cmd, &cfg)
	}
	if dockerbuild.IsContainerBackend(be) {
		return runContainerBuild(cmd, be, &cfg)
	}

	engineHash := cache.EngineKey(&cfg)
	serverHash := cache.GameServerKey(&cfg, engineHash)

	if cache.CheckSkip(cache.StageGameServer, serverHash, cfg.Game.ProjectName, noCache) {
		return nil
	}

	return runNativeBuild(cmd, &cfg, serverHash)
}

func runNativeBuild(cmd *cobra.Command, cfg *config.Config, serverHash string) error {
	enginePath := cfg.Engine.SourcePath
	if enginePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml)")
	}

	engineVersion, _ := toolchain.DetectEngineVersion(enginePath, cfg.Engine.Version)

	arch := cfg.Game.ResolvedArch()
	ddcMode, ddcPath, err := globals.ResolveDDC()
	if err != nil {
		return err
	}

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
		DDCMode:       ddcMode,
		DDCPath:       ddcPath,
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

	cache.RecordBuild(cache.StageGameServer, serverHash, globals.DryRun)

	fmt.Printf("%s server build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("\nNext: %s\n", nextAfterServerBuild())
	return nil
}

func runContainerBuild(cmd *cobra.Command, be string, cfg *config.Config) error {
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)

	if cache.CheckSkip(cache.StageGameServer, serverHash, cfg.Game.ProjectName, noCache) {
		return nil
	}

	opts, err := globals.ResolveContainerGameOptions(cfg, be)
	if err != nil {
		return err
	}
	opts.ServerTarget = cfg.Game.ResolvedServerTarget()
	opts.GameTarget = cfg.Game.ResolvedGameTarget()
	opts.SkipCook = skipCook
	opts.ServerMap = cfg.Game.ServerMap
	opts.Arch = cfg.Game.ResolvedArch()

	cli := dockerbuild.ContainerCLI(be)
	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(opts, r)

	fmt.Printf("Building %s dedicated server in %s (image: %s)...\n", cfg.Game.ProjectName, cli, opts.EngineImage)
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	cache.RecordBuild(cache.StageGameServer, serverHash, globals.DryRun)

	fmt.Printf("%s server build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("\nNext: %s\n", nextAfterServerBuild())
	return nil
}

func runClientBuild(cmd *cobra.Command, args []string) error {
	be := resolveBackend()
	if dockerbuild.IsContainerBackend(be) {
		return runContainerClientBuild(cmd, be)
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

	ddcMode, ddcPath, err := globals.ResolveDDC()
	if err != nil {
		return err
	}

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
		DDCMode:        ddcMode,
		DDCPath:        ddcPath,
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
	cache.RecordBuild(cache.StageGameClient, clientHash, globals.DryRun)

	fmt.Printf("%s client build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("Binary: %s\n", result.ClientBinary)
	fmt.Println("\nNext: ludus connect")
	return nil
}

func runContainerClientBuild(cmd *cobra.Command, be string) error {
	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)
	clientHash := cache.GameClientKey(cfg, engineHash, clientPlatform)

	if cache.CheckSkip(cache.StageGameClient, clientHash, cfg.Game.ProjectName, noCacheClient) {
		return nil
	}

	opts, err := globals.ResolveContainerGameOptions(cfg, be)
	if err != nil {
		return err
	}
	opts.ClientTarget = cfg.Game.ResolvedClientTarget()
	opts.ClientPlatform = clientPlatform
	opts.SkipCook = skipCookClient
	opts.Arch = cfg.Game.ResolvedArch()

	cli := dockerbuild.ContainerCLI(be)
	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(opts, r)

	fmt.Printf("Building %s standalone client in %s for %s (image: %s)...\n",
		cfg.Game.ProjectName, cli, clientPlatform, opts.EngineImage)
	result, err := builder.BuildClient(cmd.Context())
	if err != nil {
		return err
	}

	saveClientState(result)
	cache.RecordBuild(cache.StageGameClient, clientHash, globals.DryRun)

	fmt.Printf("%s client build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("Binary: %s\n", result.ClientBinary)
	fmt.Println("\nNext: ludus connect")
	return nil
}

func runWSL2GameBuild(cmd *cobra.Command, cfg *config.Config) error {
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	if cache.CheckSkip(cache.StageGameServer, serverHash, cfg.Game.ProjectName, noCache) {
		return nil
	}

	s, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if s.WSL2Engine == nil {
		return fmt.Errorf("no WSL2 engine build found; run: ludus engine build --backend wsl2")
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	w, err := wsl.New(r, "")
	if err != nil {
		return err
	}
	fmt.Printf("Using WSL2 distro: %s\n", w.Distro)

	ddcMode, ddcPath, err := globals.ResolveDDC()
	if err != nil {
		return err
	}

	opts := buildWSL2GameOptions(cfg, s, w, ddcMode, ddcPath)
	printBuildConfigGuidance(serverConfig)
	fmt.Printf("Building %s dedicated server in WSL2...\n", cfg.Game.ProjectName)
	result, err := wsl.BuildGame(cmd.Context(), w, opts)
	if err != nil {
		return err
	}

	cache.RecordBuild(cache.StageGameServer, serverHash, globals.DryRun)

	fmt.Printf("%s server build complete in %.0fs\n", cfg.Game.ProjectName, result.Duration)
	fmt.Printf("Output: %s\n", result.OutputDir)
	fmt.Printf("\nNext: %s\n", nextAfterServerBuild())
	return nil
}

func buildWSL2GameOptions(cfg *config.Config, s *state.State, w *wsl.WSL2, ddcMode, ddcPath string) wsl.GameOptions {
	return wsl.GameOptions{
		EnginePath:   s.WSL2Engine.EnginePath,
		ProjectPath:  cfg.Game.ProjectPath,
		ProjectName:  cfg.Game.ProjectName,
		ServerTarget: cfg.Game.ResolvedServerTarget(),
		Platform:     cfg.Game.Platform,
		Arch:         cfg.Game.ResolvedArch(),
		SkipCook:     skipCook,
		ServerMap:    cfg.Game.ServerMap,
		OutputDir:    config.ResolveServerBuildDir(cfg),
		DDCMode:      ddcMode,
		DDCPath:      resolveWSL2GameDDCPath(w, s.WSL2Engine.DDCPath, ddcMode, ddcPath),
		ServerConfig: serverConfig,
		MaxJobs:      maxJobs,
	}
}

func resolveWSL2GameDDCPath(w *wsl.WSL2, engineDDCPath, ddcMode, ddcPath string) string {
	if ddcMode == ddc.ModeLocal && engineDDCPath == "" {
		return w.ToWSLPath(ddcPath)
	}
	return engineDDCPath
}
