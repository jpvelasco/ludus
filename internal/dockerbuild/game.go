package dockerbuild

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/runner"
)

// DockerGameOptions configures a game build inside a Docker container.
type DockerGameOptions struct {
	// EngineImage is the engine Docker image to use (e.g. "ludus-engine:5.6.1").
	EngineImage string
	// ProjectPath is the host path to the .uproject file.
	// Leave empty for projects inside the engine tree (e.g. Lyra).
	ProjectPath string
	// ProjectName is the UE5 project name.
	ProjectName string
	// ServerTarget is the server build target name.
	ServerTarget string
	// ClientTarget is the client build target name.
	ClientTarget string
	// GameTarget is the default game target name.
	GameTarget string
	// Platform is the server build platform (always "Linux" for Docker builds).
	Platform string
	// ClientPlatform is the target platform for client builds.
	ClientPlatform string
	// Arch: target arch ("arm64" for Graviton). Containers run linux/amd64; arch affects UAT -platform, output dirs (LinuxArm64Server), and INI.
	Arch string
	// SkipCook skips content cooking.
	SkipCook bool
	// ServerMap is the default map for the dedicated server.
	ServerMap string
	// OutputDir is the host path where packaged output is written.
	OutputDir string
	// EngineVersion is the detected engine version (for workarounds).
	EngineVersion string
	// DDCMode is the DDC backend mode: "zen" (default), "local", or "none".
	DDCMode string
	// DDCPath is the host path for the legacy FileSystem DDC volume (mode "local").
	DDCPath string
	// DDCZenPath is the host path for the ZenStore DDC volume (mode "zen").
	// UE uses the Zen Store as its default local DDC backend from 5.4 onward;
	// cook DDC is written there. Mounting this directory at the container's
	// ZenStore data path persists it across --rm runs.
	DDCZenPath string
	// CookOnly runs only the cook step, skipping build/stage/package/archive.
	// Used for DDC warmup.
	CookOnly bool
	// Runtime is the container backend: "docker" or "podman".
	Runtime string
}

// DockerGameBuilder builds UE5 games inside Docker containers.
type DockerGameBuilder struct {
	opts   DockerGameOptions
	Runner *runner.Runner
}

// NewDockerGameBuilder creates a new Docker game builder.
func NewDockerGameBuilder(opts DockerGameOptions, r *runner.Runner) *DockerGameBuilder {
	if opts.Platform == "" {
		opts.Platform = "Linux"
	}
	return &DockerGameBuilder{opts: opts, Runner: r}
}

// Build runs the game server build inside a Docker container.
func (b *DockerGameBuilder) Build(ctx context.Context) (*game.BuildResult, error) {
	start := time.Now()
	result := &game.BuildResult{}

	if b.opts.EngineImage == "" {
		return nil, fmt.Errorf("engine Docker image not specified")
	}

	outputDir, err := b.prepareBuildContext(b.opts.OutputDir, "PackagedServer")
	if err != nil {
		return nil, err
	}
	result.OutputDir = outputDir

	if err := b.runServerBuildContainer(ctx, outputDir); err != nil {
		result.Error = err
		return result, err
	}

	result.Success = true
	platDir := config.ServerPlatformDir(b.resolveArch())
	result.OutputDir = filepath.Join(outputDir, platDir)
	result.ServerBinary = filepath.Join(outputDir, platDir, b.resolveServerTarget())
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// BuildClient runs the game client build inside a Docker container.
// Only Linux and LinuxArm64 client builds are supported in Docker (Win64 cross-compile is out of scope).
func (b *DockerGameBuilder) BuildClient(ctx context.Context) (*game.ClientBuildResult, error) {
	start := time.Now()
	result := &game.ClientBuildResult{}

	platform := b.resolveClientPlatform()
	if platform != "Linux" && platform != "LinuxArm64" {
		return nil, fmt.Errorf("Docker game builder only supports Linux client builds (got %q)", platform)
	}
	result.Platform = platform

	if b.opts.EngineImage == "" {
		return nil, fmt.Errorf("engine Docker image not specified")
	}

	outputDir, err := b.prepareBuildContext(b.opts.OutputDir, "PackagedClient")
	if err != nil {
		return nil, err
	}
	result.OutputDir = outputDir

	if err := b.runClientBuildContainer(ctx, outputDir); err != nil {
		result.Error = err
		return result, err
	}

	projectName := b.resolveProjectName()
	result.Success = true
	clientPlatform := b.resolveClientPlatform()
	binSub := "Linux"
	if clientPlatform == "LinuxArm64" {
		binSub = "LinuxArm64"
	}
	// Discover the staged client executable rather than assuming its name: UE
	// names it after the project's real client target (e.g. Lyra's
	// LyraStarterGame.uproject builds LyraGame), which need not match
	// ProjectName+"Game". Fall back to the configured client target, then the
	// ProjectName+"Game" convention.
	clientTarget := b.opts.ClientTarget
	if clientTarget == "" {
		clientTarget = projectName + "Game"
	}
	binariesDir := filepath.Join(outputDir, clientPlatform, projectName, "Binaries", binSub)
	fallback := filepath.Join(binariesDir, clientTarget)
	result.ClientBinary = config.DiscoverClientBinary(binariesDir, fallback, false)
	result.Duration = time.Since(start).Seconds()
	return result, nil
}
