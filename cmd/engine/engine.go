package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/ecr"
	engBuilder "github.com/jpvelasco/ludus/internal/engine"
	"github.com/jpvelasco/ludus/internal/prereq"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/toolchain"
	"github.com/jpvelasco/ludus/internal/wsl"
	"github.com/spf13/cobra"
)

var (
	uePath     string
	jobs       int
	backend    string
	noCache    bool
	baseImage  string
	skipEngine bool
	keepCache  bool
	wslNative  bool
	wslDistro  string
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
Use --backend podman or --backend docker to build inside a container.
Use --skip-engine to package pre-built Linux binaries without recompiling.`,
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

	buildCmd.Flags().IntVarP(&jobs, "jobs", "j", 0, "max parallel compile jobs (0 = auto-detect from CPU cores and RAM)")
	buildCmd.Flags().StringVar(&backend, "backend", "", `build backend: "native", "podman" (recommended), or "docker" (default: from ludus.yaml)`)
	buildCmd.Flags().BoolVar(&noCache, "no-cache", false, "disable build caching (forces rebuild even if inputs are unchanged)")
	buildCmd.Flags().StringVar(&baseImage, "base-image", "", "base image for container builds (default: from ludus.yaml or ubuntu:22.04)")
	buildCmd.Flags().BoolVar(&skipEngine, "skip-engine", false, "skip engine compilation; package pre-built Linux binaries into the image")
	buildCmd.Flags().BoolVar(&keepCache, "keep-cache", false, "retain BuildKit intermediate layer cache after build (default: cache is pruned to reclaim ~200 GB)")
	buildCmd.Flags().BoolVar(&wslNative, "wsl-native", false, "sync engine source to WSL2 native ext4 for faster builds")
	buildCmd.Flags().StringVar(&wslDistro, "wsl-distro", "", "WSL2 distro override (default: first running WSL2 distro)")

	Cmd.AddCommand(buildCmd)
	Cmd.AddCommand(setupCmd)
	Cmd.AddCommand(pushCmd)
}

// resolveBackend returns the effective backend, preferring CLI flag over config.
func resolveBackend() string { return globals.ResolveBackend(backend) }

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

	r := globals.NewRunner()
	return engBuilder.NewBuilder(engBuilder.BuildOptions{
		SourcePath: sourcePath,
		MaxJobs:    maxJobs,
		Verbose:    globals.Verbose,
	}, r), nil
}

func makeContainerEngineBuilder(be string) (*dockerbuild.EngineImageBuilder, error) {
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

	r := globals.NewRunner()
	return dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		SourcePath: sourcePath,
		Version:    version,
		MaxJobs:    maxJobs,
		ImageName:  imageName,
		NoCache:    noCache,
		BaseImage:  bi,
		Runtime:    be,
		SkipEngine: skipEngine,
		KeepCache:  keepCache,
		Arch:       "amd64", // force to amd64 (Epic x86_64-only toolchain); arm64 handled at game layer
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

	if err := maybeRunMacOSPreflights(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("\nNext: ludus engine build")
	return nil
}

// maybeRunMacOSPreflights runs the Linux toolchain bootstrap + GenerateProjectFiles
// inside a throwaway container when on macOS + container backend. This ensures the
// engine source tree has the Linux prerequisites even though the actual engine build
// will happen in a linux/amd64 container later.
func maybeRunMacOSPreflights(ctx context.Context) error {
	be := resolveBackend()
	if runtime.GOOS != "darwin" || !dockerbuild.IsContainerBackend(be) {
		return nil
	}

	cfg := globals.Cfg
	sourcePath := uePath
	if sourcePath == "" {
		sourcePath = cfg.Engine.SourcePath
	}
	bi := baseImage
	if bi == "" {
		bi = cfg.Engine.DockerBaseImage
	}
	version, _ := toolchain.DetectEngineVersion(sourcePath, cfg.Engine.Version)

	pfOpts := dockerbuild.MacOSPreflightOptions{
		EngineSourcePath: sourcePath,
		EngineVersion:    version,
		BaseImage:        bi,
		Runtime:          be,
		Arch:             "amd64", // force amd64 for pre-flights (Epic toolchain)
	}

	r := globals.NewRunner()
	if err := dockerbuild.RunLinuxToolchainBootstrap(ctx, pfOpts, r); err != nil {
		return fmt.Errorf("Linux toolchain bootstrap: %w", err)
	}
	if err := dockerbuild.RunLinuxGenerateProjectFiles(ctx, pfOpts, r); err != nil {
		return fmt.Errorf("Linux GenerateProjectFiles: %w", err)
	}
	return nil
}

func runBuild(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckEngineReady()); err != nil {
		return err
	}

	be := resolveBackend()
	if skipEngine && !dockerbuild.IsContainerBackend(be) {
		return fmt.Errorf("--skip-engine requires a container backend (use --backend podman or --backend docker)")
	}
	if dockerbuild.IsWSL2Backend(be) {
		return runWSL2Build(cmd)
	}
	if dockerbuild.IsContainerBackend(be) {
		return runContainerBuild(cmd, be)
	}

	return runNativeEngineBuild(cmd)
}

func runNativeEngineBuild(cmd *cobra.Command) error {
	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)

	if cache.CheckSkip(cache.StageEngine, engineHash, "Engine", noCache) {
		return nil
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

	cache.RecordBuild(cache.StageEngine, engineHash, globals.DryRun)

	fmt.Printf("Engine build complete in %.0fs at %s\n", result.Duration, result.EnginePath)
	fmt.Println("\nNext: ludus game build")
	return nil
}

func runContainerBuild(cmd *cobra.Command, be string) error {
	cfg := globals.Cfg
	engineHash := cache.EngineKey(cfg)
	cli := dockerbuild.ContainerCLI(be)

	if cache.CheckSkip(cache.StageEngine, engineHash, "Engine "+cli, noCache) {
		return nil
	}

	builder, err := makeContainerEngineBuilder(be)
	if err != nil {
		return err
	}

	if skipEngine {
		fmt.Printf("Packaging pre-built engine binaries with %s (skip-engine)...\n", cli)
	} else {
		fmt.Printf("Building Unreal Engine in %s...\n", cli)
	}
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

	cache.RecordBuild(cache.StageEngine, engineHash, globals.DryRun)

	fmt.Printf("Engine image built in %.0fs: %s\n", result.Duration, result.ImageTag)
	fmt.Printf("\nNext: ludus game build --backend %s\n", be)
	return nil
}

func runPush(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckPushReady()); err != nil {
		return err
	}

	cfg := globals.Cfg

	imageName, imageTag, err := globals.ResolveEngineImageParts(cfg)
	if err != nil {
		return err
	}

	r := globals.NewRunner()
	builder := dockerbuild.NewEngineImageBuilder(dockerbuild.EngineImageOptions{
		ImageName: imageName,
		ImageTag:  imageTag,
	}, r)

	repoName := cfg.Engine.DockerImageName
	if repoName == "" {
		repoName = "ludus-engine"
	}

	env, err := awsenv.NewResolver(globals.DryRun).Resolve(cmd.Context(), cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return err
	}

	fmt.Printf("Pushing engine image %s to ECR...\n", builder.FullImageTag())
	if err := builder.Push(cmd.Context(), ecr.PushOptions{
		ECRRepository: repoName,
		AWSRegion:     env.Region,
		AWSAccountID:  env.AccountID,
		ImageTag:      imageTag,
	}); err != nil {
		return err
	}
	pushBe := resolveBackend()
	if pushBe == "" {
		pushBe = dockerbuild.BackendDocker
	}
	fmt.Printf("\nNext: ludus game build --backend %s\n", pushBe)
	return nil
}

func runWSL2Build(cmd *cobra.Command) error {
	cfg := globals.Cfg
	sourcePath := uePath
	if sourcePath == "" {
		sourcePath = cfg.Engine.SourcePath
	}
	if sourcePath == "" {
		return fmt.Errorf("engine source path not configured (set engine.sourcePath in ludus.yaml or use --path)")
	}

	r := globals.NewRunner()
	w, err := wsl.New(r, wslDistro)
	if err != nil {
		return err
	}
	fmt.Printf("Using WSL2 distro: %s\n", w.Distro)

	version, _ := toolchain.DetectEngineVersion(sourcePath, cfg.Engine.Version)
	maxJobs := jobs
	if maxJobs == 0 {
		maxJobs = cfg.Engine.MaxJobs
	}

	enginePath, ddcPath, err := resolveWSL2EnginePaths(cmd, r, w, sourcePath, version)
	if err != nil {
		return err
	}

	result, err := wsl.BuildEngine(cmd.Context(), w, wsl.EngineOptions{
		SourcePath: sourcePath,
		MaxJobs:    maxJobs,
		WSLNative:  wslNative,
		Version:    version,
	})
	if err != nil {
		return err
	}

	saveWSL2EngineState(enginePath, ddcPath)

	fmt.Printf("Engine built in WSL2 in %.0fs: %s\n", result.Duration, enginePath)
	fmt.Println("\nNext: ludus game build --backend wsl2")
	return nil
}

// resolveWSL2EnginePaths returns the WSL2 engine and DDC paths for the build.
// When --wsl-native is set it rsyncs the source to ext4 first; otherwise it
// converts the Windows source path to a /mnt/ virtiofs path.
func resolveWSL2EnginePaths(cmd *cobra.Command, r *runner.Runner, w *wsl.WSL2, sourcePath, version string) (enginePath, ddcPath string, err error) {
	if wslNative {
		fmt.Println("Syncing engine source to WSL2 native ext4...")
		syncResult, err := wsl.SyncEngine(cmd.Context(), r, w.Distro, wsl.SyncOptions{
			SourcePath: sourcePath,
			Version:    version,
		})
		if err != nil {
			return "", "", err
		}
		fmt.Printf("Synced to %s in %.0fs\n", syncResult.WSLPath, syncResult.Duration.Seconds())
		return syncResult.WSLPath, syncResult.DDCPath, nil
	}
	ep := w.ToWSLPath(sourcePath)
	dp := w.ToWSLPath(filepath.Join(filepath.Dir(sourcePath), ".ludus", "ddc"))
	return ep, dp, nil
}

// saveWSL2EngineState persists the engine and DDC paths to .ludus/state.json.
func saveWSL2EngineState(enginePath, ddcPath string) {
	syncTime := ""
	if wslNative {
		syncTime = time.Now().UTC().Format(time.RFC3339)
	}
	if err := state.UpdateWSL2Engine(&state.WSL2EngineState{
		EnginePath: enginePath,
		IsNative:   wslNative,
		DDCPath:    ddcPath,
		SyncTime:   syncTime,
		BuiltAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}
}
