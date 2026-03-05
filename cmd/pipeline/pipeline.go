package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	ctrBuilder "github.com/devrecon/ludus/internal/container"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/dockerbuild"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	gameBuilder "github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	skipEngine    bool
	skipGame      bool
	skipContainer bool
	skipDeploy    bool
	withClient    bool
	withSession   bool
	backend       string
	noCache       bool
)

// Cmd is the full pipeline command.
var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full pipeline end-to-end",
	Long: `Executes the complete Ludus pipeline:

  1. Validate prerequisites (ludus init)
  2. Build Unreal Engine from source (ludus engine build)
  3. Build game dedicated server for Linux (ludus game build)
  4. Build Docker container image (ludus container build)  [if target requires it]
  5. Push to Amazon ECR (ludus container push)              [if target requires it]
  6. Deploy to target (ludus deploy)

Use --skip-* flags to skip stages that are already complete.
Use --backend docker to build engine and game inside Docker.
Use the global --dry-run flag to see what commands would be executed.`,
	RunE: runPipeline,
}

func init() {
	Cmd.Flags().BoolVar(&skipEngine, "skip-engine", false, "skip engine build (use existing build)")
	Cmd.Flags().BoolVar(&skipGame, "skip-game", false, "skip game build (use existing build)")
	Cmd.Flags().BoolVar(&skipContainer, "skip-container", false, "skip container build and push (use existing image)")
	Cmd.Flags().BoolVar(&skipDeploy, "skip-deploy", false, "skip deployment (build only)")
	Cmd.Flags().BoolVar(&withClient, "with-client", false, "also build a standalone Linux game client")
	Cmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	Cmd.Flags().StringVar(&backend, "backend", "", `build backend: "native" or "docker" (default: from ludus.yaml)`)
	Cmd.Flags().BoolVar(&noCache, "no-cache", false, "disable build caching (force rebuild of all stages)")
}

type pipelineStage struct {
	name string
	skip bool
	fn   func(ctx context.Context) error
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string {
	if backend != "" {
		return backend
	}
	return globals.Cfg.Engine.Backend
}

// resolveEngineImage determines the Docker image to use for game builds.
func resolveEngineImage() (string, error) {
	cfg := globals.Cfg

	if cfg.Engine.DockerImage != "" {
		return cfg.Engine.DockerImage, nil
	}

	s, err := state.Load()
	if err == nil && s.EngineImage != nil && s.EngineImage.ImageTag != "" {
		return s.EngineImage.ImageTag, nil
	}

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

func runPipeline(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg
	r := runner.NewRunner(globals.Verbose, globals.DryRun)

	projectName := cfg.Game.ProjectName
	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
	useDocker := resolveBackend() == "docker"

	// Resolve the deployment target to determine which stages are needed
	target, err := globals.ResolveTarget(cmd.Context(), cfg, "")
	if err != nil {
		return fmt.Errorf("resolving deploy target: %w", err)
	}
	caps := target.Capabilities()

	// Derive server build directory from project path
	arch := cfg.Game.ResolvedArch()
	projectPath := cfg.Game.ProjectPath
	if projectPath == "" && cfg.Game.ProjectName == "Lyra" {
		projectPath = filepath.Join(cfg.Engine.SourcePath,
			"Samples", "Games", "Lyra", "Lyra.uproject")
	}
	serverBuildDir := filepath.Join(filepath.Dir(projectPath),
		"PackagedServer", config.ServerPlatformDir(arch))

	// Compute cache hashes upfront
	engineHash := cache.EngineKey(cfg)
	serverHash := cache.GameServerKey(cfg, engineHash)
	clientHash := cache.GameClientKey(cfg, engineHash, "Linux")

	// Load cache once for pipeline-wide checks
	buildCache, _ := cache.Load()

	stages := []pipelineStage{
		{
			name: "Validate prerequisites",
			fn: func(ctx context.Context) error {
				checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
				results := checker.RunAll()
				failed := 0
				for _, res := range results {
					marker := "[OK]"
					if !res.Passed {
						marker = "[FAIL]"
						failed++
					}
					fmt.Printf("    %-6s %s\n", marker, res.Name)
				}
				if failed > 0 {
					return fmt.Errorf("%d prerequisite check(s) failed", failed)
				}
				return nil
			},
		},
		{
			name: "Build Unreal Engine",
			skip: skipEngine,
			fn: func(ctx context.Context) error {
				if !noCache && buildCache.IsHit(cache.StageEngine, engineHash) {
					fmt.Println("    Engine build is up to date (cached), skipping.")
					return nil
				}

				if useDocker {
					imageName := cfg.Engine.DockerImageName
					if imageName == "" {
						imageName = "ludus-engine"
					}

					// If a pre-built image is configured, skip engine build
					if cfg.Engine.DockerImage != "" {
						fmt.Printf("    Using pre-built engine image: %s\n", cfg.Engine.DockerImage)
						return nil
					}

					builder := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
						SourcePath: cfg.Engine.SourcePath,
						Version:    engineVersion,
						MaxJobs:    cfg.Engine.MaxJobs,
						ImageName:  imageName,
						BaseImage:  cfg.Engine.DockerBaseImage,
					}, r)
					result, err := builder.Build(ctx)
					if err != nil {
						return err
					}
					if err := state.UpdateEngineImage(&state.EngineImageState{
						ImageTag: result.ImageTag,
						BuiltAt:  time.Now().UTC().Format(time.RFC3339),
					}); err != nil {
						fmt.Printf("    Warning: failed to write state: %v\n", err)
					}
					fmt.Printf("    Engine Docker image built in %.0fs: %s\n", result.Duration, result.ImageTag)
				} else {
					builder := engBuilder.NewBuilder(engBuilder.BuildOptions{
						SourcePath: cfg.Engine.SourcePath,
						MaxJobs:    cfg.Engine.MaxJobs,
						Verbose:    globals.Verbose,
					}, r)
					result, err := builder.Build(ctx)
					if err != nil {
						return err
					}
					fmt.Printf("    Engine built in %.0fs\n", result.Duration)
				}

				buildCache.Set(cache.StageEngine, engineHash, time.Now().UTC().Format(time.RFC3339))
				_ = cache.Save(buildCache)
				return nil
			},
		},
		{
			name: fmt.Sprintf("Build %s server (%s)", projectName, config.UEPlatformName(arch)),
			skip: skipGame,
			fn: func(ctx context.Context) error {
				if !noCache && buildCache.IsHit(cache.StageGameServer, serverHash) {
					fmt.Printf("    %s server build is up to date (cached), skipping.\n", projectName)
					return nil
				}

				if useDocker {
					engineImage, err := resolveEngineImage()
					if err != nil {
						return err
					}
					builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
						EngineImage:   engineImage,
						ProjectPath:   cfg.Game.ProjectPath,
						ProjectName:   cfg.Game.ProjectName,
						ServerTarget:  cfg.Game.ResolvedServerTarget(),
						GameTarget:    cfg.Game.ResolvedGameTarget(),
						ServerMap:     cfg.Game.ServerMap,
						EngineVersion: engineVersion,
					}, r)
					result, err := builder.Build(ctx)
					if err != nil {
						return err
					}
					// Update serverBuildDir for downstream container stage
					serverBuildDir = result.OutputDir
					fmt.Printf("    %s server built in Docker in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
				} else {
					builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
						EnginePath:    cfg.Engine.SourcePath,
						ProjectPath:   cfg.Game.ProjectPath,
						ProjectName:   cfg.Game.ProjectName,
						ServerTarget:  cfg.Game.ResolvedServerTarget(),
						GameTarget:    cfg.Game.ResolvedGameTarget(),
						Platform:      cfg.Game.Platform,
						Arch:          arch,
						ServerOnly:    true,
						ServerMap:     cfg.Game.ServerMap,
						EngineVersion: engineVersion,
					}, r)
					result, err := builder.Build(ctx)
					if err != nil {
						return err
					}
					fmt.Printf("    %s server built in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
				}

				buildCache.Set(cache.StageGameServer, serverHash, time.Now().UTC().Format(time.RFC3339))
				_ = cache.Save(buildCache)
				return nil
			},
		},
		{
			name: fmt.Sprintf("Build %s client (Linux)", projectName),
			skip: !withClient,
			fn: func(ctx context.Context) error {
				if !noCache && buildCache.IsHit(cache.StageGameClient, clientHash) {
					fmt.Printf("    %s client build is up to date (cached), skipping.\n", projectName)
					return nil
				}

				if useDocker {
					engineImage, err := resolveEngineImage()
					if err != nil {
						return err
					}
					builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
						EngineImage:    engineImage,
						ProjectPath:    cfg.Game.ProjectPath,
						ProjectName:    cfg.Game.ProjectName,
						ClientTarget:   cfg.Game.ResolvedClientTarget(),
						ClientPlatform: "Linux",
						EngineVersion:  engineVersion,
					}, r)
					result, err := builder.BuildClient(ctx)
					if err != nil {
						return err
					}
					if err := state.UpdateClient(&state.ClientState{
						BinaryPath: result.ClientBinary,
						OutputDir:  result.OutputDir,
						Platform:   result.Platform,
						BuiltAt:    time.Now().UTC().Format(time.RFC3339),
					}); err != nil {
						fmt.Printf("    Warning: failed to write state: %v\n", err)
					}
					fmt.Printf("    %s client built in Docker in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
				} else {
					builder := gameBuilder.NewBuilder(gameBuilder.BuildOptions{
						EnginePath:    cfg.Engine.SourcePath,
						ProjectPath:   cfg.Game.ProjectPath,
						ProjectName:   cfg.Game.ProjectName,
						ClientTarget:  cfg.Game.ResolvedClientTarget(),
						Platform:      cfg.Game.Platform,
						EngineVersion: engineVersion,
					}, r)
					result, err := builder.BuildClient(ctx)
					if err != nil {
						return err
					}
					if err := state.UpdateClient(&state.ClientState{
						BinaryPath: result.ClientBinary,
						OutputDir:  result.OutputDir,
						BuiltAt:    time.Now().UTC().Format(time.RFC3339),
					}); err != nil {
						fmt.Printf("    Warning: failed to write state: %v\n", err)
					}
					fmt.Printf("    %s client built in %.0fs at %s\n", projectName, result.Duration, result.OutputDir)
				}

				buildCache.Set(cache.StageGameClient, clientHash, time.Now().UTC().Format(time.RFC3339))
				_ = cache.Save(buildCache)
				return nil
			},
		},
		{
			name: "Build container image",
			skip: skipContainer || !caps.NeedsContainerBuild,
			fn: func(ctx context.Context) error {
				containerHash := cache.ContainerKey(cfg, serverBuildDir)
				if !noCache && buildCache.IsHit(cache.StageContainerBuild, containerHash) {
					fmt.Println("    Container image is up to date (cached), skipping.")
					return nil
				}

				builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
					ServerBuildDir: serverBuildDir,
					ImageName:      cfg.Container.ImageName,
					Tag:            cfg.Container.Tag,
					ServerPort:     cfg.Container.ServerPort,
					ProjectName:    cfg.Game.ProjectName,
					ServerTarget:   cfg.Game.ResolvedServerTarget(),
				}, r)
				result, err := builder.Build(ctx)
				if err != nil {
					return err
				}

				buildCache.Set(cache.StageContainerBuild, containerHash, time.Now().UTC().Format(time.RFC3339))
				_ = cache.Save(buildCache)
				fmt.Printf("    Image built: %s (%.0fs)\n", result.ImageTag, result.Duration)
				return nil
			},
		},
		{
			name: "Push to Amazon ECR",
			skip: skipContainer || !caps.NeedsContainerPush,
			fn: func(ctx context.Context) error {
				builder := ctrBuilder.NewBuilder(ctrBuilder.BuildOptions{
					ImageName:  cfg.Container.ImageName,
					Tag:        cfg.Container.Tag,
					ServerPort: cfg.Container.ServerPort,
				}, r)
				return builder.Push(ctx, ctrBuilder.PushOptions{
					ECRRepository: cfg.AWS.ECRRepository,
					AWSRegion:     cfg.AWS.Region,
					AWSAccountID:  cfg.AWS.AccountID,
					ImageTag:      cfg.Container.Tag,
				})
			},
		},
		{
			name: fmt.Sprintf("Deploy to %s", target.Name()),
			skip: skipDeploy,
			fn: func(ctx context.Context) error {
				if est := pricing.FormatEstimate(cfg.GameLift.InstanceType); est != "" {
					fmt.Printf("    %s\n", est)
				}
				if sug := pricing.FormatSuggestion(cfg.GameLift.InstanceType, arch); sug != "" {
					fmt.Printf("    %s\n", sug)
				}

				imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
					cfg.AWS.AccountID, cfg.AWS.Region, cfg.AWS.ECRRepository, cfg.Container.Tag)

				result, err := target.Deploy(ctx, deploy.DeployInput{
					ImageURI:       imageURI,
					ServerBuildDir: serverBuildDir,
					ServerPort:     cfg.Container.ServerPort,
				})
				if err != nil {
					return err
				}

				if err := state.UpdateDeploy(&state.DeployState{
					TargetName: result.TargetName,
					Status:     result.Status,
					Detail:     result.Detail,
					DeployedAt: time.Now().UTC().Format(time.RFC3339),
				}); err != nil {
					fmt.Printf("    Warning: failed to write state: %v\n", err)
				}

				fmt.Printf("    Deployed to %s: %s\n", result.TargetName, result.Detail)
				return nil
			},
		},
		{
			name: "Create game session",
			skip: skipDeploy || !withSession || !caps.SupportsSession,
			fn: func(ctx context.Context) error {
				sm, ok := target.(deploy.SessionManager)
				if !ok {
					return fmt.Errorf("target %q does not support game sessions", target.Name())
				}
				info, err := sm.CreateSession(ctx, 8)
				if err != nil {
					return err
				}
				if err := state.UpdateSession(&state.SessionState{
					SessionID: info.SessionID,
					IPAddress: info.IPAddress,
					Port:      info.Port,
				}); err != nil {
					fmt.Printf("    Warning: failed to write session state: %v\n", err)
				}
				fmt.Printf("    Game session created: %s\n", info.SessionID)
				fmt.Printf("    Connect: %s:%d\n", info.IPAddress, info.Port)
				return nil
			},
		},
	}

	// Dry-run mode: print the plan, then execute with runner in dry-run mode
	if globals.DryRun {
		fmt.Println("Dry run — would execute:")
		fmt.Println()
	}

	// Execute stages
	total := len(stages)
	for i, s := range stages {
		if s.skip {
			fmt.Printf("[%d/%d] %s (skipped)\n", i+1, total, s.name)
			continue
		}

		fmt.Printf("[%d/%d] %s...\n", i+1, total, s.name)
		start := time.Now()

		if err := s.fn(cmd.Context()); err != nil {
			fmt.Printf("\nPipeline failed at stage %d/%d: %s\n", i+1, total, s.name)
			return fmt.Errorf("stage %q failed: %w", s.name, err)
		}

		elapsed := time.Since(start)
		fmt.Printf("[%d/%d] %s complete (%s)\n\n", i+1, total, s.name, elapsed.Truncate(time.Second))
	}

	fmt.Println("Pipeline complete.")
	if withSession {
		fmt.Println("\nNext: ludus connect")
	} else if !skipDeploy {
		fmt.Println("\nNext: ludus deploy session")
	}
	return nil
}
