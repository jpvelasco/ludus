package dockerbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/game"
	"github.com/devrecon/ludus/internal/runner"
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
	// SkipCook skips content cooking.
	SkipCook bool
	// ServerMap is the default map for the dedicated server.
	ServerMap string
	// OutputDir is the host path where packaged output is written.
	OutputDir string
	// EngineVersion is the detected engine version (for workarounds).
	EngineVersion string
	// DDCMode is the DDC backend mode: "local" or "none".
	DDCMode string
	// DDCPath is the host path for the local DDC volume.
	DDCPath string
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

// resolveProjectName returns the project name, defaulting to "Lyra".
func (b *DockerGameBuilder) resolveProjectName() string {
	if b.opts.ProjectName != "" {
		return b.opts.ProjectName
	}
	return "Lyra"
}

// resolveServerTarget returns the server target, defaulting to ProjectName + "Server".
func (b *DockerGameBuilder) resolveServerTarget() string {
	if b.opts.ServerTarget != "" {
		return b.opts.ServerTarget
	}
	return b.resolveProjectName() + "Server"
}

// resolveGameTarget returns the game target, defaulting to ProjectName + "Game".
func (b *DockerGameBuilder) resolveGameTarget() string {
	if b.opts.GameTarget != "" {
		return b.opts.GameTarget
	}
	return b.resolveProjectName() + "Game"
}

// isExternalProject returns true if the project is outside the engine tree
// and needs to be volume-mounted into the container.
func (b *DockerGameBuilder) isExternalProject() bool {
	return b.opts.ProjectPath != ""
}

// containerProjectPath returns the project path as seen from inside the container.
func (b *DockerGameBuilder) containerProjectPath() string {
	if b.isExternalProject() {
		return fmt.Sprintf("/project/%s.uproject", b.resolveProjectName())
	}
	// Lyra or in-engine project
	return fmt.Sprintf("/engine/Samples/Games/%s/%s.uproject",
		b.resolveProjectName(), b.resolveProjectName())
}

// generateBuildScript creates the shell script that runs inside the container.
func (b *DockerGameBuilder) generateBuildScript(serverBuild bool) string {
	script := "#!/bin/bash\nset -e\n\n"
	if serverBuild {
		script += b.serverBuildScript()
	} else {
		script += b.clientBuildScript()
	}
	return script
}

// scriptPreamble returns a standalone root-level setup script that installs
// runtime deps, creates a non-root build user, and re-execs /build.sh as that
// user. UE 5.7+ refuses to run UnrealEditor-Cmd as root on x86_64.
func (b *DockerGameBuilder) scriptPreamble() string {
	script := "#!/bin/bash\nset -e\n\n"

	// Install runtime libraries if missing. Older engine images may not include
	// them. Uses the centralized package list from deps.go.
	script += RuntimeDepsInstallScript()
	script += "\n"

	// Create non-root build user if not already in the image.
	// UE 5.7+ checks geteuid() == 0 in UnixPlatformMemory.cpp and aborts.
	script += `# Create non-root build user (UE 5.7+ refuses root on x86_64)
if ! id ue >/dev/null 2>&1; then
    useradd -m -s /bin/bash ue
    chown -R ue:ue /engine /output /ddc 2>/dev/null || true
    chown -R ue:ue /project 2>/dev/null || true
else
    # User exists (new engine image) but mounted volumes need ownership
    chown ue:ue /output /ddc 2>/dev/null || true
    chown ue:ue /project 2>/dev/null || true
    # Safety net for engine images built before the Dockerfile ownership fix.
    # New images handle this at build time; these are no-ops on current images.
    find /engine/Engine/Plugins -path '*/Build/Scripts/obj' -type d -exec chown -R ue:ue {} + 2>/dev/null || true
    chown ue:ue /engine/Engine/Binaries/Linux/*.sym 2>/dev/null || true
fi

# Re-exec the build as the ue user, preserving container env vars (-p).
# Override HOME because su -p keeps HOME=/root from the container's root user,
# and .NET SDK / UE tools write to $HOME/.dotnet, $HOME/.local, etc.
exec su -p ue -c "export HOME=/home/ue && cd /engine && bash /build.sh"
`
	return script
}

// envArgs returns extra container -e flags for environment variables that must
// survive the preamble's su user switch (container-level env vars persist).
func (b *DockerGameBuilder) envArgs() []string {
	var args []string
	v := b.opts.EngineVersion
	if v == "" || v == "5.6" {
		args = append(args, "-e", "NuGetAuditLevel=critical")
	}
	return args
}

// serverBuildScript returns the shell commands for a server build inside Docker.
func (b *DockerGameBuilder) serverBuildScript() string {
	projectPath := b.containerProjectPath()

	if b.opts.CookOnly {
		script := "cd /engine\n\n"
		args := fmt.Sprintf(`bash Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project="%s" \
  -platform=Linux \
  -server -noclient \
  -cook -skipbuild \
  -NoCompileEditor -NoP4 \
  -map=MinimalDefaultMap`,
			projectPath)
		return script + args + "\n"
	}

	serverTarget := b.resolveServerTarget()
	gameTarget := b.resolveGameTarget()

	script := fmt.Sprintf(`# Ensure DefaultServerTarget in DefaultEngine.ini
INI_PATH="%s/Config/DefaultEngine.ini"
if [ -f "$INI_PATH" ] && ! grep -q "DefaultServerTarget" "$INI_PATH"; then
    if grep -q "DefaultGameTarget=%s" "$INI_PATH"; then
        sed -i "s/DefaultGameTarget=%s/DefaultGameTarget=%s\nDefaultServerTarget=%s/" "$INI_PATH"
        echo "Set DefaultServerTarget=%s in $INI_PATH"
    fi
fi

`, filepath.Dir(projectPath), gameTarget, gameTarget, gameTarget, serverTarget, serverTarget)

	script += "cd /engine\n\n"

	args := fmt.Sprintf(`bash Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project="%s" \
  -platform=Linux \
  -server -noclient \
  -servertargetname=%s \
  -build -stage -package -archive \
  -archivedirectory="/output"`,
		projectPath, serverTarget)

	if !b.opts.SkipCook {
		args += " \\\n  -cook"
	} else {
		args += " \\\n  -skipcook"
	}

	if b.opts.ServerMap != "" {
		args += fmt.Sprintf(` \
  -map="%s"`, b.opts.ServerMap)
	}

	return script + args + "\n"
}

// clientBuildScript returns the shell commands for a client build inside Docker.
func (b *DockerGameBuilder) clientBuildScript() string {
	projectPath := b.containerProjectPath()

	platform := b.opts.ClientPlatform
	if platform == "" {
		platform = "Linux"
	}
	clientTarget := b.opts.ClientTarget
	if clientTarget == "" {
		clientTarget = b.resolveProjectName() + "Game"
	}

	script := "cd /engine\n\n"

	args := fmt.Sprintf(`bash Engine/Build/BatchFiles/RunUAT.sh BuildCookRun \
  -project="%s" \
  -platform=%s \
  -build -stage -package -archive \
  -archivedirectory="/output"`,
		projectPath, platform)

	if !b.opts.SkipCook {
		args += " \\\n  -cook"
	} else {
		args += " \\\n  -skipcook"
	}

	_ = clientTarget // target name is implicit in the project for client builds
	return script + args + "\n"
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
	result.OutputDir = filepath.Join(outputDir, "LinuxServer")
	result.ServerBinary = filepath.Join(outputDir, "LinuxServer", b.resolveServerTarget())
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// prepareBuildContext resolves the output directory to an absolute path and
// creates it if needed. defaultSubdir is used when OutputDir is empty.
func (b *DockerGameBuilder) prepareBuildContext(outputDir, defaultSubdir string) (string, error) {
	if outputDir == "" {
		outputDir = filepath.Join(".", defaultSubdir)
	}
	if !filepath.IsAbs(outputDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting working directory: %w", err)
		}
		outputDir = filepath.Join(cwd, outputDir)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}
	return outputDir, nil
}

// runServerBuildContainer writes the build script and runs the Docker container.
func (b *DockerGameBuilder) runServerBuildContainer(ctx context.Context, outputDir string) error {
	buildScript := b.generateBuildScript(true)
	cli := ContainerCLI(b.opts.Runtime)
	return b.runBuildContainer(ctx, outputDir, buildScript, cli+" game build")
}

// runBuildContainer mounts a preamble script and a build script into a container.
// The preamble runs as root (installs deps, creates non-root user), then re-execs
// the build script as the ue user via su -p.
func (b *DockerGameBuilder) runBuildContainer(ctx context.Context, outputDir, script, label string) error {
	preamble := b.scriptPreamble()

	preambleFile, err := os.CreateTemp("", "ludus-preamble-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp preamble script: %w", err)
	}
	defer os.Remove(preambleFile.Name())

	if _, err := preambleFile.WriteString(preamble); err != nil {
		preambleFile.Close()
		return fmt.Errorf("writing preamble script: %w", err)
	}
	preambleFile.Close()

	buildFile, err := os.CreateTemp("", "ludus-build-*.sh")
	if err != nil {
		return fmt.Errorf("creating temp build script: %w", err)
	}
	defer os.Remove(buildFile.Name())

	if _, err := buildFile.WriteString(script); err != nil {
		buildFile.Close()
		return fmt.Errorf("writing build script: %w", err)
	}
	buildFile.Close()

	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/output", outputDir),
		"-v", fmt.Sprintf("%s:/preamble.sh:ro", preambleFile.Name()),
		"-v", fmt.Sprintf("%s:/build.sh:ro", buildFile.Name()),
	}

	if b.isExternalProject() {
		projectDir := filepath.Dir(b.opts.ProjectPath)
		args = append(args, "-v", fmt.Sprintf("%s:/project", projectDir))
	}

	ddcExtra, err := b.ddcArgs()
	if err != nil {
		return err
	}
	args = append(args, ddcExtra...)
	args = append(args, b.envArgs()...)

	args = append(args, b.opts.EngineImage, "bash", "/preamble.sh")

	cli := ContainerCLI(b.opts.Runtime)
	if err := b.Runner.Run(ctx, cli, args...); err != nil {
		return fmt.Errorf("%s failed: %w", label, err)
	}
	return nil
}

// ddcArgs returns the extra container args (volume mounts and env vars) for the
// configured DDC mode. It also creates the local DDC directory if needed.
func (b *DockerGameBuilder) ddcArgs() ([]string, error) {
	switch b.opts.DDCMode {
	case "local":
		if b.opts.DDCPath == "" {
			return nil, fmt.Errorf("DDC mode is 'local' but no path configured; set ddc.localPath in ludus.yaml or use --ddc none")
		}
		if err := os.MkdirAll(b.opts.DDCPath, 0755); err != nil {
			return nil, fmt.Errorf("creating DDC directory: %w", err)
		}
		fmt.Printf("DDC: local (persistent at %s)\n", b.opts.DDCPath)
		return []string{
			"-v", fmt.Sprintf("%s:/ddc", b.opts.DDCPath),
			"-e", ddc.EnvOverride("/ddc"),
		}, nil
	case "none":
		fmt.Println("DDC: disabled")
		return nil, nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported DDC mode %q; valid values are \"local\" or \"none\"", b.opts.DDCMode)
	}
}

// BuildClient runs the game client build inside a Docker container.
// Only Linux client builds are supported in Docker (Win64 cross-compile is out of scope).
func (b *DockerGameBuilder) BuildClient(ctx context.Context) (*game.ClientBuildResult, error) {
	start := time.Now()
	result := &game.ClientBuildResult{}

	platform := b.resolveClientPlatform()
	if platform != "Linux" {
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
	result.ClientBinary = filepath.Join(outputDir, "Linux", projectName, "Binaries", "Linux", projectName+"Game")
	result.Duration = time.Since(start).Seconds()
	return result, nil
}

// resolveClientPlatform returns the client platform, defaulting to "Linux".
func (b *DockerGameBuilder) resolveClientPlatform() string {
	if b.opts.ClientPlatform != "" {
		return b.opts.ClientPlatform
	}
	return "Linux"
}

// runClientBuildContainer writes the client build script and runs the Docker container.
func (b *DockerGameBuilder) runClientBuildContainer(ctx context.Context, outputDir string) error {
	buildScript := b.generateBuildScript(false)
	cli := ContainerCLI(b.opts.Runtime)
	return b.runBuildContainer(ctx, outputDir, buildScript, cli+" client build")
}
