package engine

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/dockerbuild"
	engBuilder "github.com/devrecon/ludus/internal/engine"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	uePath  string
	jobs    int
	backend string
	noCache bool
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
	buildCmd.Flags().BoolVar(&noCache, "no-cache", false, "disable Docker build cache (only for docker backend)")

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

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	return dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		SourcePath: sourcePath,
		Version:    version,
		MaxJobs:    maxJobs,
		ImageName:  imageName,
		NoCache:    noCache,
	}, r), nil
}

func runSetup(cmd *cobra.Command, args []string) error {
	builder, err := makeBuilder()
	if err != nil {
		return err
	}

	fmt.Println("Running engine setup (Setup.sh)...")
	return builder.Setup(cmd.Context())
}

func runBuild(cmd *cobra.Command, args []string) error {
	if resolveBackend() == "docker" {
		return runDockerBuild(cmd)
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

	fmt.Printf("Engine build complete in %.0fs at %s\n", result.Duration, result.EnginePath)
	return nil
}

func runDockerBuild(cmd *cobra.Command) error {
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

	fmt.Printf("Engine Docker image built in %.0fs: %s\n", result.Duration, result.ImageTag)
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
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
	return builder.Push(cmd.Context(), dockerbuild.PushOptions{
		ECRRepository: repoName,
		AWSRegion:     cfg.AWS.Region,
		AWSAccountID:  cfg.AWS.AccountID,
		ImageTag:      imageTag,
	})
}
