package dockerbuild

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
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
