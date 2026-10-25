package dockerbuild

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// writeStagedExecutable creates path (and its parent directories) as a file.
func writeStagedExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestBuildServerBinaryResolvesPackagedExecutable covers #718 for the
// container builder: the server build result must point at the executable UE
// actually stages — under the packaged project directory (the .uproject
// basename, which may differ from ProjectName) and Binaries/<platform> — and
// discover the Shipping-suffixed filename for Shipping builds instead of
// returning the bare target directly under the platform directory.
func TestBuildServerBinaryResolvesPackagedExecutable(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "LinuxServer", "ExampleContent", "Binaries", "Linux", "ExampleServer-Linux-Shipping")
	writeStagedExecutable(t, bin)

	opts := DockerGameOptions{
		EngineImage:  "test:tag",
		ProjectPath:  testsupport.FakeProject(t, "ExampleContent"),
		ProjectName:  "Example",
		ServerTarget: "ExampleServer",
		ServerConfig: "Shipping",
		OutputDir:    root,
	}
	res, err := NewDockerGameBuilder(opts, runner.NewRunner(false, true)).Build(context.Background())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if res.ServerBinary != bin {
		t.Errorf("Build() ServerBinary = %q, want %q", res.ServerBinary, bin)
	}
}

// TestBuildClientBinarySearchesUprojectDir covers #717 for the container
// builder: client discovery must search the packaged project directory named
// after the .uproject filename (ExampleContent), not ProjectName (Example).
func TestBuildClientBinarySearchesUprojectDir(t *testing.T) {
	tests := []struct {
		name   string
		staged bool
	}{
		{name: "discovers executable under uproject basename", staged: true},
		{name: "fallback path under uproject basename", staged: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "Linux", "ExampleContent", "Binaries", "Linux", "ExampleGame")
			if tt.staged {
				writeStagedExecutable(t, bin)
			}

			opts := DockerGameOptions{
				EngineImage:  "test:tag",
				ProjectPath:  testsupport.FakeProject(t, "ExampleContent"),
				ProjectName:  "Example",
				ClientTarget: "ExampleGame",
				OutputDir:    root,
			}
			res, err := NewDockerGameBuilder(opts, runner.NewRunner(false, true)).BuildClient(context.Background())
			if err != nil {
				t.Fatalf("BuildClient() error = %v", err)
			}
			if res.ClientBinary != bin {
				t.Errorf("BuildClient() ClientBinary = %q, want %q", res.ClientBinary, bin)
			}
		})
	}
}
