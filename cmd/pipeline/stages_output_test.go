package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// seedPackagedServer creates the packaged server directory for cfg so a game
// build cache hit passes its output validation (#739).
func seedPackagedServer(t *testing.T, cfg *config.Config) {
	t.Helper()
	if err := os.MkdirAll(config.ResolveServerBuildDir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestStageGameBuildCacheHitRequiresOutput pins #739 for the pipeline game
// stage: a warm entry whose packaged output is gone must rerun the build.
func TestStageGameBuildCacheHitRequiresOutput(t *testing.T) {
	projectPath := testsupport.FakeProject(t, "TestGame")
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "C:/ue5", Version: "5.7.3"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: projectPath, Platform: "Linux"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	bc := newTestCache()
	serverKey := "server-hash"
	bc.Set(cache.StageGameServer, serverKey, "2026-01-01T00:00:00Z")

	p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "native", withBuildCache: true})
	p.serverHash = serverKey
	p.buildCache = bc
	p.r, _ = testsupport.RecordingRunner()
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	p.cfg.Engine.SourcePath = engineRoot
	// No packaged server directory on disk: the hit must be discarded (#739).

	output := captureStdout(func() {
		if err := p.stageGameBuild(t.Context()); err != nil {
			t.Fatalf("stageGameBuild() error = %v, want nil", err)
		}
	})
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("cache hit honored with missing packaged output: %s", output)
	}
	if !strings.Contains(output, "cache hit ignored: output missing") {
		t.Errorf("output = %q, want the ignored-hit notice", output)
	}
}

// TestStageGameBuildCacheHitHonoredWithOutput pins #739's positive path: with
// the packaged output present, the warm entry skips the build.
func TestStageGameBuildCacheHitHonoredWithOutput(t *testing.T) {
	projectPath := testsupport.FakeProject(t, "TestGame")
	cfg := &config.Config{
		Engine: config.EngineConfig{SourcePath: "C:/ue5", Version: "5.7.3"},
		Game:   config.GameConfig{ProjectName: "TestGame", ProjectPath: projectPath, Platform: "Linux"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	seedPackagedServer(t, cfg)

	bc := newTestCache()
	serverKey := "server-hash"
	bc.Set(cache.StageGameServer, serverKey, "2026-01-01T00:00:00Z")

	p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "native", withBuildCache: true})
	p.serverHash = serverKey
	p.buildCache = bc

	output := captureStdout(func() {
		if err := p.stageGameBuild(t.Context()); err != nil {
			t.Fatalf("stageGameBuild() error = %v, want nil", err)
		}
	})
	if !strings.Contains(output, "(cached), skipping") {
		t.Errorf("output = %q, want the cache hit to skip the build", output)
	}
}

// TestPipelineServerOptsThreadBuildConfig pins #696's pipeline half: the
// server build options carry the same resolved configuration the cache key
// hashes, so the build and its key can never disagree.
func TestPipelineServerOptsThreadBuildConfig(t *testing.T) {
	cfg := pipelineOptsCfg()
	globals.SetGlobals(t, cfg)

	t.Run("native server", func(t *testing.T) {
		p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "native"})
		if got := p.nativeServerOpts("TestGame").ServerConfig; got != "Development" {
			t.Errorf("nativeServerOpts().ServerConfig = %q, want Development", got)
		}
	})

	t.Run("container server", func(t *testing.T) {
		p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "podman"})
		opts, err := p.containerServerOpts()
		if err != nil {
			t.Fatalf("containerServerOpts() error = %v", err)
		}
		if got := opts.ServerConfig; got != "Development" {
			t.Errorf("containerServerOpts().ServerConfig = %q, want Development", got)
		}
	})
}

// TestStageContainerBuildImageNameChange pins #692 for the pipeline: changing
// container.imageName must invalidate the container stage's cache entry.
func TestStageContainerBuildImageNameChange(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
			ProjectPath: filepath.Join(t.TempDir(), "P", "P.uproject"),
		},
		Container: config.ContainerConfig{ImageName: "image-one", Tag: "latest"},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{
		Stdout: "Platforms: linux/amd64, linux/arm64",
	})

	// Warm entry for image-one; the stage runs with image-two.
	buildDir := resolveServerBuildDir(cfg, "amd64")
	warm := cfg.Clone()
	bc := newTestCache()
	bc.Set(cache.StageContainerBuild, cache.ContainerKey(&warm, buildDir), "2026-01-01T00:00:00Z")

	cfg.Container.ImageName = "image-two"
	p := newTestPipelineCtx(t, cfg, &testContextOpts{withBuildCache: true})
	p.buildCache = bc
	p.serverBuildDir = buildDir
	p.r, _ = testsupport.RecordingRunner()

	output := captureStdout(func() {
		if err := p.stageContainerBuild(t.Context()); err != nil {
			t.Fatalf("stageContainerBuild() error = %v, want nil", err)
		}
	})
	if strings.Contains(output, "(cached), skipping") {
		t.Errorf("image-two stage reused image-one's cache entry: %s", output)
	}
}

// TestPipelineEngineOutputValidator covers the engine stage's per-backend
// output validator (#739): native builds validate the host Binaries dir,
// container builds the built image ref, and prebuilt/WSL2 builds carry none.
func TestPipelineEngineOutputValidator(t *testing.T) {
	t.Run("native validates host binaries", func(t *testing.T) {
		cfg := &config.Config{Engine: config.EngineConfig{SourcePath: t.TempDir()}}
		globals.SetGlobals(t, cfg)
		p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "native"})
		got := p.engineOutput(t.Context())
		if got == nil {
			t.Fatal("engineOutput() = nil for a native build, want a validator")
		}
		if got() {
			t.Error("engineOutput() = true before any engine build, want false")
		}
	})

	t.Run("container validates built image", func(t *testing.T) {
		cfg := &config.Config{
			Engine: config.EngineConfig{SourcePath: t.TempDir(), Version: "5.7.3"},
			Game:   config.GameConfig{ProjectName: "TestGame"},
		}
		globals.SetGlobals(t, cfg, globals.WithDryRun(true))
		p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "docker"})
		if ref := p.engineImageRef(); !strings.Contains(ref, "ludus-engine:5.7.3") {
			t.Errorf("engineImageRef() = %q, want the ludus-engine:5.7.3 tag", ref)
		}
		got := p.engineOutput(t.Context())
		if got == nil {
			t.Fatal("engineOutput() = nil for a container build, want an image probe")
		}
	})

	t.Run("prebuilt image has no validator", func(t *testing.T) {
		cfg := &config.Config{Engine: config.EngineConfig{DockerImage: "my.repo/engine:5.7.3"}}
		globals.SetGlobals(t, cfg)
		p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "docker"})
		if got := p.engineOutput(t.Context()); got != nil {
			t.Errorf("engineOutput() = %v for a prebuilt image, want nil", got)
		}
	})

	t.Run("wsl2 has no validator", func(t *testing.T) {
		cfg := &config.Config{Engine: config.EngineConfig{Backend: "wsl2"}}
		globals.SetGlobals(t, cfg)
		p := newTestPipelineCtx(t, cfg, &testContextOpts{containerBackend: "wsl2"})
		if got := p.engineOutput(t.Context()); got != nil {
			t.Errorf("engineOutput() = %v for a WSL2 build, want nil", got)
		}
	})
}
