package pipeline

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestStageContainerBuildCacheHit(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		Container: config.ContainerConfig{
			ImageName: "test-image",
			Tag:       "latest",
		},
	}

	// Dry-run keeps the image-existence probe (docker image inspect) offline:
	// the runner echoes the probe and honors the hit (#739).
	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	bc := newTestCache()
	containerHash := cache.ContainerKey(cfg, "C:/build/out")
	bc.Set(cache.StageContainerBuild, containerHash, "2024-01-01T00:00:00Z")

	p := &pipelineCtx{
		cfg:            cfg,
		r:              globals.NewRunner(),
		arch:           "amd64",
		serverBuildDir: "C:/build/out",
		buildCache:     bc,
		target:         &stubTarget{},
	}

	err := p.stageContainerBuild(context.Background())
	if err != nil {
		t.Fatalf("stageContainerBuild() error = %v, want nil", err)
	}
}

func TestStageContainerBuildDryRun(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "latest",
			ServerPort: 7777,
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// On a cross-arch build (target arch != runtime.GOARCH, e.g. amd64 on the
	// arm64 macOS runner) internal/container shells out to
	// `docker buildx inspect --bootstrap` before any dry-run guard and requires
	// the target platform in its output. The probe does not run on an amd64 host,
	// so stub docker with both platforms listed to keep the test host-independent
	// — the macOS runner has no docker on PATH at all.
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{
		Stdout: "Platforms: linux/amd64, linux/arm64",
	})

	r, getLines := testsupport.RecordingRunner()

	bc := newTestCache()

	p := &pipelineCtx{
		cfg:            cfg,
		r:              r,
		arch:           "amd64",
		serverBuildDir: filepath.Join(t.TempDir(), "out"),
		buildCache:     bc,
		target:         &stubTarget{},
	}

	if err := p.stageContainerBuild(context.Background()); err != nil {
		t.Fatalf("stageContainerBuild() error = %v, want nil", err)
	}

	lines := getLines()
	for _, want := range []string{"docker build", "--platform linux/amd64", "-t test-image:latest"} {
		if !findInLines(lines, want) {
			t.Errorf("recorded commands missing %q, got: %v", want, lines)
		}
	}
}

// TestStageContainerBuildPodmanBackend is the issue-691 regression: with the
// engine/game backend set to podman, the deployment image build must use
// podman, not silently fall back to docker.
func TestStageContainerBuildPodmanBackend(t *testing.T) {
	cfg := &config.Config{
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "latest",
			ServerPort: 7777,
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	// On a cross-arch host the build probes the runtime with `buildx inspect`
	// before any dry-run guard; stub podman with both platforms so the test
	// stays host-independent (mirrors the docker stub above).
	testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{
		Stdout: "Platforms: linux/amd64, linux/arm64",
	})

	r, getLines := testsupport.RecordingRunner()

	p := &pipelineCtx{
		cfg:              cfg,
		r:                r,
		arch:             "amd64",
		containerBackend: "podman",
		serverBuildDir:   filepath.Join(t.TempDir(), "out"),
		buildCache:       newTestCache(),
		target:           &stubTarget{},
	}

	if err := p.stageContainerBuild(context.Background()); err != nil {
		t.Fatalf("stageContainerBuild() error = %v, want nil", err)
	}

	lines := getLines()
	if !findInLines(lines, "podman build") {
		t.Errorf("podman backend must build with podman, got: %v", lines)
	}
	if findInLines(lines, "docker build") {
		t.Errorf("podman backend must not fall back to docker, got: %v", lines)
	}
}

// TestContainerBuildOptionsPackagedDirName is the issue-690 regression: the
// packaged content directory is named after the .uproject file, not after
// game.projectName, and the pipeline must pass the resolved name through just
// like the standalone container command.
func TestContainerBuildOptionsPackagedDirName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		projectPath string
		want        string
	}{
		{
			name:        "uproject basename differs from project name",
			projectName: "Example",
			projectPath: filepath.Join(t.TempDir(), "ExampleStarter.uproject"),
			want:        "ExampleStarter",
		},
		{
			name:        "falls back to project name without project path",
			projectName: "Example",
			want:        "Example",
		},
		{
			name: "defaults to Lyra",
			want: "Lyra",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Game: config.GameConfig{
					ProjectName: tt.projectName,
					ProjectPath: tt.projectPath,
				},
			}
			p := &pipelineCtx{cfg: cfg}

			if got := p.containerBuildOptions().PackagedDirName; got != tt.want {
				t.Errorf("PackagedDirName = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestContainerBuildOptionsBackend is the issue-691 regression: the image build
// honors the selected container runtime. Engine/game-only backends (native,
// wsl2) resolve to the image-build default, which the builder treats as docker.
func TestContainerBuildOptionsBackend(t *testing.T) {
	tests := []struct {
		name             string
		containerBackend string
		want             string
	}{
		{name: "podman backend", containerBackend: "podman", want: "podman"},
		{name: "docker backend", containerBackend: "docker", want: "docker"},
		{name: "native backend defaults", containerBackend: "native", want: ""},
		{name: "wsl2 backend defaults", containerBackend: "wsl2", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Game: config.GameConfig{ProjectName: "TestGame"}}
			globals.SetGlobals(t, cfg)

			p := &pipelineCtx{cfg: cfg, containerBackend: tt.containerBackend}

			if got := p.containerBuildOptions().Backend; got != tt.want {
				t.Errorf("Backend = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestContainerBuildOptionsPassthrough pins the remaining option fields the
// pipeline forwards to the image build.
func TestContainerBuildOptionsPassthrough(t *testing.T) {
	cfg := &config.Config{
		Container: config.ContainerConfig{
			ImageName:  "test-image",
			Tag:        "v1.0",
			ServerPort: 7777,
		},
		Game: config.GameConfig{
			ProjectName:  "TestGame",
			ProjectPath:  filepath.Join(t.TempDir(), "TestGame.uproject"),
			ServerTarget: "TestGameServer",
		},
	}
	globals.SetGlobals(t, cfg)

	buildDir := filepath.Join(t.TempDir(), "out")
	p := &pipelineCtx{cfg: cfg, arch: "arm64", serverBuildDir: buildDir}

	opts := p.containerBuildOptions()
	want := []struct {
		field string
		got   string
		want  string
	}{
		{"ServerBuildDir", opts.ServerBuildDir, buildDir},
		{"ImageName", opts.ImageName, "test-image"},
		{"Tag", opts.Tag, "v1.0"},
		{"ServerTarget", opts.ServerTarget, "TestGameServer"},
		{"Arch", opts.Arch, "arm64"},
	}
	for _, tt := range want {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
		}
	}
	if opts.ServerPort != 7777 {
		t.Errorf("ServerPort = %d, want 7777", opts.ServerPort)
	}
}

func TestStageContainerPushDryRun(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			ImageName: "test-image",
			Tag:       "v1.0",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r, _ := testsupport.RecordingRunner()

	globals.SwapResolveTarget(t, func(ctx context.Context, c *config.Config, s string) (deploy.Target, error) {
		return &stubTarget{}, nil
	})

	p := &pipelineCtx{
		cfg:    cfg,
		r:      r,
		arch:   "amd64",
		target: &stubTarget{},
	}

	err := p.stageContainerPush(context.Background())
	if err != nil {
		t.Fatalf("stageContainerPush() error = %v, want nil", err)
	}
}

func TestBuildImageURINoRepo(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "",
		},
		Container: config.ContainerConfig{
			Tag: "v1.0",
		},
	}

	globals.SetGlobals(t, cfg)

	p := newTestPipelineCtx(t, cfg, nil)

	_, err := p.buildImageURI(context.Background())
	// Should fail due to missing repository
	if err == nil {
		t.Errorf("buildImageURI() with missing repository expected error, got nil")
	}
}

func TestBuildImageURINoTag(t *testing.T) {
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "my-game",
		},
		Container: config.ContainerConfig{
			Tag: "",
		},
	}

	globals.SetGlobals(t, cfg)

	p := newTestPipelineCtx(t, cfg, nil)

	_, err := p.buildImageURI(context.Background())
	// Should fail due to missing tag
	if err == nil {
		t.Errorf("buildImageURI() with missing tag expected error, got nil")
	}
}

func TestBaseDockerGameOptsSuccess(t *testing.T) {
	engineRoot, projectPath, cfg := setupTestContext(t, "TestGame")

	cfg.Engine.DockerImageName = "my-engine"
	cfg.Container.Tag = "v1.0"

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	p := newTestPipelineCtx(t, cfg, &testContextOpts{
		containerBackend: "docker",
		fullVersion:      "5.7.3",
		ddcMode:          "zen",
		ddcPath:          "C:/ludus/ddc",
		ddcZenPath:       "/home/ue/.config/Epic/UnrealEngine/Common/Zen/Data",
	})

	opts, err := p.baseDockerGameOpts()
	if err != nil {
		t.Fatalf("baseDockerGameOpts() error = %v, want nil", err)
	}
	if opts.EngineImage == "" {
		t.Errorf("baseDockerGameOpts() EngineImage is empty")
	}

	_ = engineRoot
	_ = projectPath
}
