package globals

import (
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/toolchain"
)

// ResolveDDCMode returns the effective DDC mode.
// CLI flag (DDCMode) takes precedence over config (Cfg.DDC.Mode).
// Returns an error for invalid mode values.
func ResolveDDCMode() (string, error) {
	var mode string
	if DDCMode != "" {
		mode = DDCMode
	} else if Cfg != nil && Cfg.DDC.Mode != "" {
		mode = Cfg.DDC.Mode
	}
	return ddc.ValidateDDCMode(mode)
}

// WarnIfLegacyDDC prints the legacy-DDC deprecation notice to stderr when the
// effective mode resolves to "local". Written to stderr so it never corrupts
// --json or mcp stdout. Called once per invocation from config load
// (PersistentPreRunE), so it covers every command (including ddc status).
// ludus doctor reports the same guidance as a structured diagnostic via
// checkDDCMode. Invalid modes are ignored here (validated elsewhere with a
// proper error).
func WarnIfLegacyDDC() {
	mode, err := ResolveDDCMode()
	if err == nil && mode == ddc.ModeLocal {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", ddc.LocalModeDeprecationWarning)
	}
}

// ResolveDDCPath returns the effective DDC host path.
// Config localPath takes precedence over the default path (~/.ludus/ddc).
// Validates that the path is absolute (relative paths break Docker volume mounts).
func ResolveDDCPath() (string, error) {
	if Cfg != nil && Cfg.DDC.LocalPath != "" {
		return ddc.ResolvePath(Cfg.DDC.LocalPath)
	}
	return ddc.DefaultPath()
}

// ResolveZenPath returns the effective ZenStore host path.
// Config zenPath takes precedence over the default (~/.ludus/zen).
func ResolveZenPath() (string, error) {
	if Cfg != nil && Cfg.DDC.ZenPath != "" {
		return ddc.ResolveZenPath(Cfg.DDC.ZenPath)
	}
	return ddc.DefaultZenPath()
}

// ResolveDDC returns the effective DDC mode, filesystem path, and ZenStore path.
// When mode is "none", both paths are returned empty (DDC is disabled).
func ResolveDDC() (mode, path, zenPath string, err error) {
	mode, err = ResolveDDCMode()
	if err != nil {
		return "", "", "", err
	}
	if mode == ddc.ModeNone {
		return mode, "", "", nil
	}
	path, err = ResolveDDCPath()
	if err != nil {
		return "", "", "", fmt.Errorf("resolving DDC path: %w", err)
	}
	zenPath, err = ResolveZenPath()
	if err != nil {
		return "", "", "", fmt.Errorf("resolving DDC zen path: %w", err)
	}
	return mode, path, zenPath, nil
}

// ResolveEngineImage determines the Docker image to use for builds.
// Precedence: config DockerImage > state EngineImage > constructed from config.
// When requireVersion is true, returns an error if the engine version cannot
// be detected (used by DDC warmup where "latest" is not meaningful).
func ResolveEngineImage(cfg *config.Config, requireVersion bool) (string, error) {
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
	version := cfg.Engine.Version
	if version == "" {
		if requireVersion {
			return "", fmt.Errorf("could not detect engine version (source_path=%q, version=%q); set engine.version or engine.docker_image in ludus.yaml",
				cfg.Engine.SourcePath, cfg.Engine.Version)
		}
		version = "latest"
	}
	return fmt.Sprintf("%s:%s", imageName, version), nil
}

// BaseDockerGameOptions returns a DockerGameOptions populated with the common
// fields shared by all container game builds (server, client, warmup).
// Callers set build-specific fields (ServerTarget, ClientTarget, CookOnly, etc.)
// on the returned struct before passing it to NewDockerGameBuilder.
func BaseDockerGameOptions(cfg *config.Config, engineImage, engineVersion, ddcMode, ddcPath, ddcZenPath, runtime string) dockerbuild.DockerGameOptions {
	return dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		EngineVersion: engineVersion,
		// Arch comes from the canonical config resolution so container builds
		// select the requested platform (arm64 Graviton) instead of silently
		// defaulting to amd64 (#698).
		Arch:       cfg.Game.ResolvedArch(),
		DDCMode:    ddcMode,
		DDCPath:    ddcPath,
		DDCZenPath: ddcZenPath,
		Runtime:    runtime,
		// OutputDir is the PackagedServer archive root, derived from projectPath
		// so the game build writes where container build reads. The Docker game
		// builder appends the platform subdirectory itself, so this must be the
		// root (not ResolveServerBuildDir, which already includes the platform —
		// passing that would double it to .../PackagedServer/LinuxServer/LinuxServer).
		// Client builds override this with the PackagedClient root
		// (ApplyClientBuildOptions). Without a root, the builder defaults to
		// ./PackagedServer relative to cwd.
		OutputDir: config.ResolveServerArchiveRoot(cfg),
	}
}

// ApplyServerBuildOptions sets the server-build-specific fields on opts from
// cfg plus the requested build configuration. serverConfig may be empty,
// which defers to UAT's default (Development).
func ApplyServerBuildOptions(opts dockerbuild.DockerGameOptions, cfg *config.Config, serverConfig string) dockerbuild.DockerGameOptions {
	opts.ServerTarget = cfg.Game.ResolvedServerTarget()
	opts.GameTarget = cfg.Game.ResolvedGameTarget()
	opts.SkipCook = cfg.Game.SkipCook
	opts.ServerMap = cfg.Game.ServerMap
	opts.ServerConfig = serverConfig
	return opts
}

// ApplyClientBuildOptions sets the client-build-specific fields on opts from
// cfg. The archive root is the PackagedClient directory — mirroring the
// native client builder's layout — so client builds archive next to, not
// inside, the packaged server (#750).
func ApplyClientBuildOptions(opts dockerbuild.DockerGameOptions, cfg *config.Config, platform string) dockerbuild.DockerGameOptions {
	opts.ClientTarget = cfg.Game.ResolvedClientTarget()
	opts.ClientPlatform = platform
	opts.SkipCook = cfg.Game.SkipCook
	opts.OutputDir = config.ResolveClientArchiveRoot(cfg)
	return opts
}

// ResolveContainerGameOptions resolves the engine image, engine version, and DDC
// configuration, then returns a fully populated DockerGameOptions for the given
// runtime backend. Callers still set build-specific fields (ServerTarget,
// ClientTarget, CookOnly, SkipCook, etc.) after calling this function.
func ResolveContainerGameOptions(cfg *config.Config, be string) (dockerbuild.DockerGameOptions, error) {
	engineImage, err := ResolveEngineImage(cfg, false)
	if err != nil {
		return dockerbuild.DockerGameOptions{}, err
	}

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	ddcMode, ddcPath, ddcZenPath, err := ResolveDDC()
	if err != nil {
		return dockerbuild.DockerGameOptions{}, fmt.Errorf("resolving DDC config: %w", err)
	}

	return BaseDockerGameOptions(cfg, engineImage, engineVersion, ddcMode, ddcPath, ddcZenPath, be), nil
}

// ResolveContainerServerBuildOptions resolves the shared container options and
// applies the server build settings (targets, cook, map, build configuration).
// It is the single assembly for container server builds across CLI, MCP, and
// the pipeline (#730).
func ResolveContainerServerBuildOptions(cfg *config.Config, be, serverConfig string) (dockerbuild.DockerGameOptions, error) {
	opts, err := ResolveContainerGameOptions(cfg, be)
	if err != nil {
		return dockerbuild.DockerGameOptions{}, err
	}
	return ApplyServerBuildOptions(opts, cfg, serverConfig), nil
}

// ResolveContainerClientBuildOptions resolves the shared container options and
// applies the client build settings (target, platform, cook, PackagedClient
// archive root). It is the single assembly for container client builds across
// CLI, MCP, and the pipeline (#750).
func ResolveContainerClientBuildOptions(cfg *config.Config, be, platform string) (dockerbuild.DockerGameOptions, error) {
	opts, err := ResolveContainerGameOptions(cfg, be)
	if err != nil {
		return dockerbuild.DockerGameOptions{}, err
	}
	return ApplyClientBuildOptions(opts, cfg, platform), nil
}
