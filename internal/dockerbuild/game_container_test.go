package dockerbuild

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestPrepareBuildContext(t *testing.T) {
	tests := []struct {
		name          string
		outputDir     func(tmp string) string
		defaultSubdir string
		wantDir       func(tmp string) string
	}{
		{
			name:          "empty outputDir defaults to defaultSubdir under cwd",
			outputDir:     func(string) string { return "" },
			defaultSubdir: "PackagedServer",
			wantDir:       func(tmp string) string { return filepath.Join(tmp, "PackagedServer") },
		},
		{
			name:          "relative outputDir resolved against cwd",
			outputDir:     func(string) string { return "out" },
			defaultSubdir: "PackagedServer",
			wantDir:       func(tmp string) string { return filepath.Join(tmp, "out") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewDockerGameBuilder(DockerGameOptions{}, runner.NewRunner(false, true))
			tmp := t.TempDir()
			t.Chdir(tmp)

			got, err := b.prepareBuildContext(tt.outputDir(tmp), tt.defaultSubdir)
			assertPreparedBuildContext(t, got, err, tt.wantDir(tmp))
		})
	}

	t.Run("absolute outputDir used as-is", func(t *testing.T) {
		b := NewDockerGameBuilder(DockerGameOptions{}, runner.NewRunner(false, true))
		abs := filepath.Join(t.TempDir(), "Packaged")

		got, err := b.prepareBuildContext(abs, "PackagedServer")
		assertPreparedBuildContext(t, got, err, abs)
	})
}

func assertPreparedBuildContext(t *testing.T, got string, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("prepareBuildContext() error: %v", err)
	}
	if got != want {
		t.Errorf("prepareBuildContext() = %q, want %q", got, want)
	}
	if info, statErr := os.Stat(got); statErr != nil || !info.IsDir() {
		t.Errorf("expected %q to be created as a directory", got)
	}
}

// TestRunBuildContainer exercises runBuildContainer with a dry-run Runner, so
// no docker/podman invocation actually happens; this covers the preamble/build
// script tempfile writes and chmods without needing a container runtime.
func TestRunBuildContainer(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewDockerGameBuilder(DockerGameOptions{EngineImage: "ludus-engine:test"}, r)

	if err := b.runBuildContainer(context.Background(), t.TempDir(), "#!/bin/bash\necho hi\n", "docker game build"); err != nil {
		t.Fatalf("runBuildContainer() error: %v", err)
	}
}

func TestRunServerBuildContainer(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewDockerGameBuilder(DockerGameOptions{EngineImage: "ludus-engine:test"}, r)

	if err := b.runServerBuildContainer(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("runServerBuildContainer() error: %v", err)
	}
}

func TestRunClientBuildContainer(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewDockerGameBuilder(DockerGameOptions{EngineImage: "ludus-engine:test"}, r)

	if err := b.runClientBuildContainer(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("runClientBuildContainer() error: %v", err)
	}
}

func TestBuild_EngineImageRequired(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewDockerGameBuilder(DockerGameOptions{}, r)

	if _, err := b.Build(context.Background()); err == nil || !strings.Contains(err.Error(), "engine Docker image not specified") {
		t.Errorf("expected engine image error, got %v", err)
	}
}

func TestBuildClient_UnsupportedPlatform(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewDockerGameBuilder(DockerGameOptions{ClientPlatform: "Win64"}, r)

	if _, err := b.BuildClient(context.Background()); err == nil || !strings.Contains(err.Error(), "only supports Linux client builds") {
		t.Errorf("expected unsupported platform error, got %v", err)
	}
}

func TestBuildClient_EngineImageRequired(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewDockerGameBuilder(DockerGameOptions{ClientPlatform: "Linux"}, r)

	if _, err := b.BuildClient(context.Background()); err == nil || !strings.Contains(err.Error(), "engine Docker image not specified") {
		t.Errorf("expected engine image error, got %v", err)
	}
}

func TestPrepareBuildContext_MkdirAllError(t *testing.T) {
	// A file in place of an ancestor dir makes os.MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	b := NewDockerGameBuilder(DockerGameOptions{}, runner.NewRunner(false, true))

	if _, err := b.prepareBuildContext(filepath.Join(blocker, "out"), "PackagedServer"); err == nil {
		t.Error("expected error when output parent is a file")
	}
}

// silentRunner returns a non-dry-run runner with output discarded, so stubbed
// container CLIs actually execute without polluting test output.
func silentRunner() *runner.Runner {
	r := runner.NewRunner(false, false)
	r.Stdout = io.Discard
	r.Stderr = io.Discard
	return r
}

// TestRunBuildContainerExternalProject covers the isExternalProject branch of
// runBuildContainer (game_container.go:87-90): a project outside the engine
// tree adds the /project volume mount, and the stubbed docker CLI exits 0.
func TestRunBuildContainerExternalProject(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{})
	b := NewDockerGameBuilder(DockerGameOptions{
		EngineImage: "ludus-engine:test",
		ProjectPath: `C:\outside\MyGame\MyGame.uproject`,
	}, silentRunner())

	if err := b.runBuildContainer(context.Background(), t.TempDir(), "#!/bin/bash\necho hi\n", "docker game build"); err != nil {
		t.Fatalf("runBuildContainer(external project) error = %v", err)
	}
}

// TestRunBuildContainerRunFailure covers the Runner failure branch of
// runBuildContainer (game_container.go:102-104): the stubbed docker CLI exits
// non-zero, so the failure is wrapped with the build label.
func TestRunBuildContainerRunFailure(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	b := NewDockerGameBuilder(DockerGameOptions{
		EngineImage: "ludus-engine:test",
	}, silentRunner())

	err := b.runBuildContainer(context.Background(), t.TempDir(), "#!/bin/bash\necho hi\n", "docker game build")
	if err == nil || !strings.Contains(err.Error(), "docker game build failed") {
		t.Fatalf("runBuildContainer() error = %v, want 'docker game build failed'", err)
	}
}
