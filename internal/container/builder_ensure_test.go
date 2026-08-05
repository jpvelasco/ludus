package container

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/ecr"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// stubCachedWrapper points the wrapper cache at a fresh temp home and pre-stages
// a fake wrapper binary, so EnsureBinary returns the cached path without cloning,
// downloading, or invoking any external tool.
func stubCachedWrapper(t *testing.T, arch string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheDir := filepath.Join(home, ".cache", "ludus", "game-server-wrapper")
	bin := filepath.Join(cacheDir, "out", "linux", arch, "gamelift-servers-managed-containers",
		"amazon-gamelift-servers-game-server-wrapper")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("fake wrapper binary"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateAndWriteDockerfile_WritesAndCleans(t *testing.T) {
	stubCachedWrapper(t, "amd64")
	dir := t.TempDir()
	b := NewBuilder(BuildOptions{
		ServerBuildDir:  dir,
		ImageName:       "ludus-server",
		Tag:             "latest",
		ServerPort:      7777,
		ProjectName:     "MyGame",
		ServerTarget:    "MyGameServer",
		PackagedDirName: "MyGame",
	}, runner.NewRunner(false, false))

	cleanup, err := b.generateAndWriteDockerfile(context.Background())
	if err != nil {
		t.Fatalf("generateAndWriteDockerfile() error = %v", err)
	}

	want := []string{"Dockerfile", ".dockerignore", "config.yaml",
		"amazon-gamelift-servers-game-server-wrapper"}
	for _, f := range want {
		if _, statErr := os.Stat(filepath.Join(dir, f)); statErr != nil {
			t.Errorf("expected %s to be staged into the build dir", f)
		}
	}

	cleanup()
	for _, f := range want {
		if _, statErr := os.Stat(filepath.Join(dir, f)); statErr == nil {
			t.Errorf("cleanup should have removed %s", f)
		}
	}
}

// TestGenerateAndWriteDockerfile_EnsureWrapperError forces EnsureBinary to reach
// `go build` (all earlier stages pre-satisfied) and fail via a stubbed go.
func TestGenerateAndWriteDockerfile_EnsureWrapperError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheDir := filepath.Join(home, ".cache", "ludus", "game-server-wrapper")
	if err := os.MkdirAll(filepath.Join(cacheDir, "src", "ext", "gamelift-servers-server-sdk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "gamelift-servers-server-sdk.zip"), []byte("zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	testsupport.FakeTool(t, "go", testsupport.ToolBehavior{ExitCode: 1})

	b := NewBuilder(BuildOptions{
		ServerBuildDir: t.TempDir(),
		ProjectName:    "MyGame",
		ServerTarget:   "MyGameServer",
	}, runner.NewRunner(false, false))

	_, err := b.generateAndWriteDockerfile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "game server wrapper") {
		t.Fatalf("error = %v, want wrapper build failure", err)
	}
}

func TestGenerateAndWriteDockerfile_WriteErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		conflict string
		wantErr  string
	}{
		{"wrapper copy", "amazon-gamelift-servers-game-server-wrapper", "copying wrapper binary"},
		{"wrapper config", "config.yaml", "writing wrapper config"},
		{"dockerfile", "Dockerfile", "writing Dockerfile"},
		{"dockerignore", ".dockerignore", "writing .dockerignore"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubCachedWrapper(t, "amd64")
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, tt.conflict), 0o755); err != nil {
				t.Fatal(err)
			}
			b := NewBuilder(BuildOptions{
				ServerBuildDir: dir,
				ProjectName:    "MyGame",
				ServerTarget:   "MyGameServer",
			}, runner.NewRunner(false, false))

			_, err := b.generateAndWriteDockerfile(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCheckContainerPlatformSupport(t *testing.T) {
	for _, tt := range []struct {
		name     string
		behavior testsupport.ToolBehavior
		platform string
		wantErr  bool
	}{
		{"buildx unavailable", testsupport.ToolBehavior{ExitCode: 1}, "linux/arm64", true},
		{"platform supported", testsupport.ToolBehavior{Stdout: "Platforms: linux/amd64, linux/arm64"}, "linux/arm64", false},
		{"platform unsupported", testsupport.ToolBehavior{Stdout: "Platforms: linux/amd64"}, "linux/arm64", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "docker", tt.behavior)
			err := checkContainerPlatformSupport("docker", tt.platform)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuild_MissingServerDir(t *testing.T) {
	b := NewBuilder(BuildOptions{}, runner.NewRunner(false, true))
	res, err := b.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "server build directory not specified") {
		t.Fatalf("Build() error = %v, want missing-dir error", err)
	}
	if res == nil || res.Error == nil {
		t.Fatal("expected result.Error to be set")
	}
}

func TestBuild_GenerateAndWriteFailure(t *testing.T) {
	stubCachedWrapper(t, "amd64")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := NewBuilder(BuildOptions{
		ServerBuildDir: dir,
		ImageName:      "img",
		Tag:            "t",
	}, runner.NewRunner(false, false))

	res, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("Build() expected error from wrapper config write")
	}
	if res == nil || res.Error == nil {
		t.Fatal("expected result.Error to be set")
	}
}

func TestBuild_DockerBuildFailure(t *testing.T) {
	stubCachedWrapper(t, runtime.GOARCH)
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	b := NewBuilder(BuildOptions{
		ServerBuildDir: t.TempDir(),
		ImageName:      "img",
		Tag:            "t",
		Arch:           runtime.GOARCH,
	}, runner.NewRunner(false, false))

	res, err := b.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("Build() error = %v, want docker build failure", err)
	}
	if res == nil || res.Error == nil {
		t.Fatal("expected result.Error to be set")
	}
}

func TestPush_DryRun(t *testing.T) {
	r := runner.NewRunner(false, true)
	b := NewBuilder(BuildOptions{ImageName: "ludus-server", Tag: "v1.0", ServerPort: 7777}, r)

	err := b.Push(context.Background(), ecr.PushOptions{
		ECRRepository: "ludus-server",
		AWSRegion:     "us-east-1",
		AWSAccountID:  "123456789012",
		ImageTag:      "v1.0",
	})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
}

func TestRunDockerBuild_CrossArchAndNoCache(t *testing.T) {
	other := otherArch()
	for _, tt := range []struct {
		name     string
		backend  string
		noCache  bool
		wantArgs []string
	}{
		{"docker cross-arch no-cache", "docker", true, []string{"--platform", "linux/" + other, "--provenance=false", "--no-cache"}},
		{"docker cross-arch cached", "docker", false, []string{"--platform", "linux/" + other, "--provenance=false"}},
		{"podman cross-arch", "podman", false, []string{"--platform", "linux/" + other}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, tt.backend, testsupport.ToolBehavior{
				Stdout: "Platforms: linux/amd64, linux/arm64",
			})
			r, getLines := testsupport.RecordingRunner()
			b := NewBuilder(BuildOptions{
				ServerBuildDir: t.TempDir(),
				ImageName:      "img",
				Tag:            "t",
				Arch:           other,
				Backend:        tt.backend,
				NoCache:        tt.noCache,
			}, r)

			if err := b.runDockerBuild(context.Background(), "img:t"); err != nil {
				t.Fatalf("runDockerBuild() error = %v", err)
			}
			for _, want := range tt.wantArgs {
				if !lineContains(getLines(), want) {
					t.Errorf("recorded commands missing %q, got: %v", want, getLines())
				}
			}
		})
	}
}

func TestRunDockerBuild_CrossArchProbeFailure(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	b := NewBuilder(BuildOptions{
		ServerBuildDir: t.TempDir(),
		ImageName:      "img",
		Tag:            "t",
		Arch:           otherArch(),
	}, runner.NewRunner(false, true))

	err := b.runDockerBuild(context.Background(), "img:t")
	if err == nil || !strings.Contains(err.Error(), "buildx not available") {
		t.Fatalf("runDockerBuild() error = %v, want buildx probe failure", err)
	}
}

func TestResolveServerBinaryName_MissingBinDir(t *testing.T) {
	b := NewBuilder(BuildOptions{
		ServerBuildDir: t.TempDir(),
		ProjectName:    "MyGame",
		ServerTarget:   "MyGameServer",
	}, nil)
	if got := b.resolveServerBinaryName(); got != "MyGameServer" {
		t.Errorf("got %q, want %q", got, "MyGameServer")
	}
}

func TestCopyFile_DirectorySourceFails(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(sub, filepath.Join(dir, "out.bin")); err == nil {
		t.Error("expected error copying a directory as a file")
	}
}

// otherArch returns the architecture that differs from the host, so cross-arch
// code paths run identically on amd64 and arm64 test hosts.
func otherArch() string {
	if runtime.GOARCH == "arm64" {
		return "amd64"
	}
	return "arm64"
}

// lineContains reports whether any echoed command line contains want.
func lineContains(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}
