package dockerbuild

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

// UBA needs >= 1 GB of /dev/shm for in-memory file mapping; Docker's default
// is 64 MB, so every build container must request more (#840).
func TestBuildContainersPassShmSize(t *testing.T) {
	for _, tt := range []struct{ name, shmSize, want string }{
		{"default", "", "--shm-size " + DefaultShmSize},
		{"configured", "8g", "--shm-size 8g"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for cmd, out := range map[string]string{
				"engine build": engineBuildCommand(t, tt.shmSize),
				"game run":     gameRunCommand(t, tt.shmSize),
			} {
				if !strings.Contains(out, tt.want) {
					t.Errorf("%s command missing %q, got: %s", cmd, tt.want, out)
				}
			}
		})
	}
}

func dryRunRunner() (*runner.Runner, *bytes.Buffer) {
	r := runner.NewRunner(false, true)
	var buf bytes.Buffer
	r.Stdout = &buf
	return r, &buf
}

func engineBuildCommand(t *testing.T, shmSize string) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "Setup.sh"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, buf := dryRunRunner()
	b := NewEngineImageBuilder(EngineImageOptions{SourcePath: src, Runtime: "docker", ShmSize: shmSize}, r)
	if _, err := b.Build(context.Background()); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return buf.String()
}

func gameRunCommand(t *testing.T, shmSize string) string {
	t.Helper()
	r, buf := dryRunRunner()
	b := NewDockerGameBuilder(DockerGameOptions{EngineImage: "ludus-engine:test", ShmSize: shmSize}, r)
	if err := b.runBuildContainer(context.Background(), t.TempDir(), "#!/bin/bash\n", "docker game build"); err != nil {
		t.Fatalf("runBuildContainer() error = %v", err)
	}
	return buf.String()
}
