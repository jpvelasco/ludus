package engine

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/ecr"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	uePath    string
	jobs      int
	backend   string
	noCache   bool
	baseImage string
)

// Cmd is the top-level engine command group.
var Cmd = &cobra.Command{
	Use:   "engine",
	Short: "Build and manage Unreal Engine from source",
	Long: `Commands for building Unreal Engine from source. This handles running
Setup.sh to download dependencies, generating project files, and compiling
the engine for the target platform.`,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Unreal Engine from source",
	Long: `Runs the full engine build pipeline:

  1. Run Setup.sh to download dependencies
  2. Generate project files
  3. Compile the engine (Development Editor + Server targets)

Use --jobs to control build parallelism (lower values use less memory).
Use --backend docker to build inside a Docker container.`,
	RunE: runBuild,
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run Setup.sh to download engine dependencies",
	RunE:  runSetup,
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push engine Docker image to Amazon ECR",
	Long: `Pushes the engine Docker image to Amazon ECR. The image must have been
previously built with 'ludus engine build --backend docker'.

The ECR repository is created automatically if it does not exist.`,
	RunE: runPush,
}

func init() {
	Cmd.PersistentFlags().StringVar(&uePath, "path", "", "path to Unreal Engine source (default: from ludus.yaml)")

	buildCmd.Flags().IntVarP(&jobs, "jobs", "j", 0, "max parallel compile jobs (0 = auto-detect based on available RAM)")
	buildCmd.Flags().StringVar(&backend, "backend", "", `build backend: "native" or "docker" (default: from ludus.yaml)`)
	buildCmd.Flags().BoolVar(&noCache, "no-cache", false, "disable build caching (forces rebuild even if inputs are unchanged)")
	buildCmd.Flags().StringVar(&baseImage, "base-image", "", "Docker base image for engine builds (default: from ludus.yaml or ubuntu:22.04)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(setupCmd)
	Cmd.AddCommand(pushCmd)
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string {
	if backend != "" {
		return backend
	}
	return globals.Cfg.Engine.Backend
}

func makeBuilder() (*engBuilder.Builder, error) {
	cfg := globals.Cfg
	sourcePath := uePath
	if sourcePath == "" {
		sourcePath = cfg.Engine.SourcePath
	}
	if sourcePath == "" {
		return nil, fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml or use --path)")
	}

	maxJobs := jobs
	if maxJobs == 0 {
		maxJobs = cfg.Engine.MaxJobs
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	return engBuilder.NewBuilder(engBuilder.BuildOptions{
		SourcePath: sourcePath,
		MaxJobs:    maxJobs,
		Verbose:    globals.Verbose,
	}, r), nil
}

func makeDockerEngineBuilder() (*dockerbuild.EngineImageBuilder, error) {
	cfg := globals.Cfg
	sourcePath := uePath
	if sourcePath == "" {
		sourcePath = cfg.Engine.SourcePath
	}
	if sourcePath == "" {
		return nil, fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml or use --path)")
	}

	maxJobs := jobs
	if maxJobs == 0 {
		maxJobs = cfg.Engine.MaxJobs
	}

	version, _ := toolchain.DetectEngineVersion(sourcePath, cfg.Engine.Version)
	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}

	bi := baseImage
	if bi == "" {
		bi = cfg.Engine.DockerBaseImage
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	return dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		SourcePath: sourcePath,
		Version:    version,
		MaxJobs:    maxJobs,
		ImageName:  imageName,
		NoCache:    noCache,
		BaseImage:  bi,
	}, r), nil
}

func runSetup(cmd *cobra.Command, args []string) error {
	builder, err := makeBuilder()
	if err != nil {
		return err
	}

	fmt.Println("Running engine setup...")
	if err := builder.Setup(cmd.Context()); err != nil {
		return err
	}
	fmt.Println("\nNext: ludus engine build")
	return nil
}

func runBuild(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckEngineReady()); err != nil {
		return err
	}

	if resolveBackend() == "docker" {
		return runDockerBuild(cmd)
	}

	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)

	if !noCache {
		c, err := cache.Load()
		if err == nil {
			if c.IsHit(cache.StageEngine, engineHash) {
				fmt.Println("Engine build is up to date (cached), skipping.")
				return nil
			}
			if reason := c.MissReason(cache.StageEngine, engineHash); reason != "" {
				fmt.Printf("Cache: %s\n", reason)
			}
		}
	}

	builder, err := makeBuilder()
	if err != nil {
		return err
	}

	fmt.Println("Building Unreal Engine from source...")
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	// Update cache on success
	if c, cErr := cache.Load(); cErr == nil {
		c.Set(cache.StageEngine, engineHash, time.Now().UTC().Format(time.RFC3339))
		_ = cache.Save(c)
	}

	fmt.Printf("Engine build complete in %.0fs at %s\n", result.Duration, result.EnginePath)
	fmt.Println("\nNext: ludus game build")
	return nil
}

func runDockerBuild(cmd *cobra.Command) error {
	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)

	if !noCache {
		c, err := cache.Load()
		if err == nil {
			if c.IsHit(cache.StageEngine, engineHash) {
				fmt.Println("Engine Docker build is up to date (cached), skipping.")
				return nil
			}
			if reason := c.MissReason(cache.StageEngine, engineHash); reason != "" {
				fmt.Printf("Cache: %s\n", reason)
			}
		}
	}

	builder, err := makeDockerEngineBuilder()
	if err != nil {
		return err
	}

	fmt.Println("Building Unreal Engine in Docker...")
	result, err := builder.Build(cmd.Context())
	if err != nil {
		return err
	}

	// Persist engine image info to state
	if err := state.UpdateEngineImage(&state.EngineImageState{
		ImageTag: result.ImageTag,
		BuiltAt:  time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	// Update cache on success
	if c, cErr := cache.Load(); cErr == nil {
		c.Set(cache.StageEngine, engineHash, time.Now().UTC().Format(time.RFC3339))
		_ = cache.Save(c)
	}

	fmt.Printf("Engine Docker image built in %.0fs: %s\n", result.Duration, result.ImageTag)
	fmt.Println("\nNext: ludus game build --backend docker")
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckPushReady()); err != nil {
		return err
	}

	cfg := globals.Cfg

	// Resolve the engine image tag from state or config
	imageTag := ""
	imageName := cfg.Engine.DockerImageName
	if imageName == "" {
		imageName = "ludus-engine"
	}

	s, err := state.Load()
	if err == nil && s.EngineImage != nil {
		imageTag = s.EngineImage.ImageTag
	}

	if imageTag == "" {
		// Construct from config
		version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
		tag := version
		if tag == "" {
			tag = "latest"
		}
		imageTag = fmt.Sprintf("%s:%s", imageName, tag)
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		ImageName: imageName,
	}, r)

	// Use a dedicated ECR repo for the engine image (default: image name)
	repoName := imageName

	fmt.Printf("Pushing engine image %s to ECR...\n", imageTag)
	if err := builder.Push(cmd.Context(), ecr.PushOptions{
		ECRRepository: repoName,
		AWSRegion:     cfg.AWS.Region,
		AWSAccountID:  cfg.AWS.AccountID,
		ImageTag:      imageTag,
	}); err != nil {
		return err
	}
	fmt.Println("\nNext: ludus game build --backend docker")
	return nil
}
