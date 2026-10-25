package game

import (
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/wsl"
)

// containerClientBuildConfig returns a config that routes the client build
// through a container backend, with a real (temp) project path so archive
// roots resolve deterministically.
func containerClientBuildConfig(t *testing.T) *config.Config {
	t.Helper()
	projectPath := testsupport.FakeProject(t, "TestGame")
	return &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  "C:/ue5",
			Version:     "5.7.3",
			DockerImage: "my.repo/engine:5.7.3",
		},
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ProjectPath:  projectPath,
			ClientTarget: "TestGameGame",
		},
	}
}

// TestRunContainerClientBuildArchivesToClientRoot pins #750: a container
// client build must mount the PackagedClient archive root at /output, not the
// PackagedServer root. The dry-run docker command line exposes the mount.
func TestRunContainerClientBuildArchivesToClientRoot(t *testing.T) {
	cfg := containerClientBuildConfig(t)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	t.Chdir(t.TempDir())

	origBackend, origPlatform := backend, clientPlatform
	backend, clientPlatform = "docker", "Linux"
	t.Cleanup(func() { backend, clientPlatform = origBackend, origPlatform })

	output := captureGameStdout(t, func() {
		if err := runContainerClientBuild(gameCmd(t), "docker"); err != nil {
			t.Fatalf("runContainerClientBuild() error = %v", err)
		}
	})

	clientRoot := config.ResolveClientArchiveRoot(cfg)
	if !strings.Contains(output, clientRoot+":/output") {
		t.Errorf("docker mount should use the PackagedClient root %q, got:\n%s", clientRoot, output)
	}
	if serverRoot := config.ResolveServerArchiveRoot(cfg); strings.Contains(output, serverRoot+":/output") {
		t.Errorf("docker mount should not use the PackagedServer root %q, got:\n%s", serverRoot, output)
	}
}

// TestBuildWSL2GameOptionsOutputDir pins #751: the CLI WSL2 game build hands
// the builder the PackagedServer archive ROOT. Passing the platform-qualified
// build dir would make UAT double the platform folder
// (.../PackagedServer/LinuxServer/LinuxServer).
func TestBuildWSL2GameOptionsOutputDir(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: `C:\ue5`},
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: "/game/TestGame.uproject",
			Arch:        "arm64",
		},
	}
	s := &state.State{WSL2Engine: &state.WSL2EngineState{EnginePath: "/mnt/c/ue5"}}

	opts := buildWSL2GameOptions(cfg, s, &wsl.WSL2{}, "none", "")

	want := config.ResolveServerArchiveRoot(cfg)
	if opts.OutputDir != want {
		t.Errorf("buildWSL2GameOptions().OutputDir = %q, want archive root %q", opts.OutputDir, want)
	}
	if strings.HasSuffix(opts.OutputDir, "LinuxArm64Server") {
		t.Errorf("OutputDir %q must not include the platform subdirectory", opts.OutputDir)
	}
}
