package container

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/cache"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// init gives the never-executed package-level commands a non-nil context, since
// cobra v1.10.2 returns c.ctx directly (nil until the command is run) and the
// awsenv resolver passes it straight into the AWS SDK.
func init() {
	buildCmd.SetContext(context.Background())
	pushCmd.SetContext(context.Background())
}

// containerBuildConfig returns a config whose server build dir resolves into a
// fresh temp project tree, matching the host architecture to skip the
// cross-arch emulation probe.
func containerBuildConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	projDir := t.TempDir()
	cfg := &config.Config{
		Engine: config.EngineConfig{
			Backend:    "docker",
			SourcePath: filepath.Join(projDir, "Engine"),
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "MyGame",
			ProjectPath: filepath.Join(projDir, "MyGame", "MyGame.uproject"),
			Arch:        runtime.GOARCH,
		},
		Container: config.ContainerConfig{
			ImageName:  "ludus-server",
			Tag:        "latest",
			ServerPort: 7777,
		},
	}
	serverBuildDir := config.ResolveServerBuildDir(cfg)
	if err := os.MkdirAll(serverBuildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return cfg, serverBuildDir
}

// stubBuildPrereqTools stubs the tools prereq.CheckDockerReady and the built-in
// Dockerfile linter look up, keeping runBuild fully offline and deterministic.
func stubBuildPrereqTools(t *testing.T) {
	t.Helper()
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker":   {},
		"go":       {Stdout: "go version go1.25.12 windows/amd64"},
		"hadolint": {},
	})
}

// stubAWSEnv points the AWS SDK at empty shared config/credentials files so
// awsenv resolution never parses the host's ~/.aws config (which can carry
// profile sources the SDK mishandles) and never reads real credentials.
func stubAWSEnv(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
}

// resetContainerFlags restores the package-level command flags after each test.
func resetContainerFlags(t *testing.T) {
	t.Helper()
	prevTag, prevPushTag, prevNoCache, prevArch, prevBackend := tag, pushTag, noCache, archFlag, backend
	t.Cleanup(func() {
		tag, pushTag, noCache, archFlag, backend = prevTag, prevPushTag, prevNoCache, prevArch, prevBackend
	})
	tag, pushTag, noCache, archFlag, backend = "", "", false, "", ""
}

// captureContainerStdout runs fn while redirecting stdout to a pipe and returns
// the captured output plus fn's error.
func captureContainerStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := run()
	_ = w.Close()
	os.Stdout = prev
	t.Cleanup(func() { os.Stdout = prev })
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), runErr
}

func TestRunBuildDryRun(t *testing.T) {
	for _, tt := range []struct {
		name     string
		archFlag string
		backend  string
	}{
		{"default backend and arch", "", ""},
		{"explicit arch flag", runtime.GOARCH, ""},
		{"podman backend", "", "podman"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetContainerFlags(t)
			cfg, _ := containerBuildConfig(t)
			globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithVerbose(true), globals.WithNoLogs(true))
			stubBuildPrereqTools(t)
			t.Chdir(t.TempDir())
			archFlag = tt.archFlag
			backend = tt.backend

			output, err := captureContainerStdout(t, func() error { return runBuild(buildCmd, nil) })
			if err != nil {
				t.Fatalf("runBuild() error = %v", err)
			}
			for _, want := range []string{"Building container image...", "Container image built", "Next: ludus container push"} {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunBuildCacheHit(t *testing.T) {
	resetContainerFlags(t)
	cfg, serverBuildDir := containerBuildConfig(t)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
	stubBuildPrereqTools(t)
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash := cache.ContainerKey(cfg, serverBuildDir)
	data, err := json.Marshal(&cache.Cache{Entries: map[cache.StageKey]*cache.Entry{
		cache.StageContainerBuild: {Hash: hash, BuiltAt: "2026-01-01T00:00:00Z"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ludus", "cache.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := captureContainerStdout(t, func() error { return runBuild(buildCmd, nil) })
	if err != nil {
		t.Fatalf("runBuild() error = %v", err)
	}
	if !strings.Contains(output, "(cached), skipping") {
		t.Errorf("expected cache-hit skip message, got:\n%s", output)
	}
}

func TestRunBuildPrereqFailure(t *testing.T) {
	resetContainerFlags(t)
	cfg, _ := containerBuildConfig(t)
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 1},
		"go":     {Stdout: "go version go1.25.12 windows/amd64"},
	})
	t.Chdir(t.TempDir())

	if err := runBuild(buildCmd, nil); err == nil {
		t.Fatal("runBuild() expected prereq failure")
	}
}

func TestRunBuildBuildError(t *testing.T) {
	resetContainerFlags(t)
	cfg := &config.Config{
		Engine: config.EngineConfig{Backend: "docker", Version: "5.7.3"},
		Game:   config.GameConfig{ProjectName: "MyGame", Arch: runtime.GOARCH},
		Container: config.ContainerConfig{
			ImageName:  "ludus-server",
			Tag:        "latest",
			ServerPort: 7777,
		},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
	stubBuildPrereqTools(t)
	t.Chdir(t.TempDir())

	if err := runBuild(buildCmd, nil); err == nil {
		t.Fatal("runBuild() expected build error")
	}
}

func TestRunPushDryRun(t *testing.T) {
	for _, tt := range []struct {
		name    string
		pushTag string
	}{
		{"default tag from config", ""},
		{"explicit push tag", "v1.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetContainerFlags(t)
			cfg := &config.Config{
				AWS: config.AWSConfig{
					AccountID:     "123456789012",
					Region:        "us-east-1",
					ECRRepository: "ludus-server",
				},
				Container: config.ContainerConfig{
					ImageName:  "ludus-server",
					Tag:        "latest",
					ServerPort: 7777,
				},
			}
			globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithVerbose(true), globals.WithNoLogs(true))
			stubAWSEnv(t)
			testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
				"docker": {},
				"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
			})
			t.Chdir(t.TempDir())
			pushTag = tt.pushTag

			output, err := captureContainerStdout(t, func() error { return runPush(pushCmd, nil) })
			if err != nil {
				t.Fatalf("runPush() error = %v", err)
			}
			for _, want := range []string{"Pushing container image to ECR...", "Next: ludus deploy fleet"} {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunPushPrereqFailure(t *testing.T) {
	resetContainerFlags(t)
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "ludus-server",
		},
		Container: config.ContainerConfig{ImageName: "ludus-server", Tag: "latest", ServerPort: 7777},
	}
	globals.SetGlobals(t, cfg, globals.WithDryRun(true), globals.WithNoLogs(true))
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 1},
		"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
	})
	t.Chdir(t.TempDir())

	if err := runPush(pushCmd, nil); err == nil {
		t.Fatal("runPush() expected prereq failure")
	}
}

func TestRunPushECRError(t *testing.T) {
	resetContainerFlags(t)
	cfg := &config.Config{
		AWS: config.AWSConfig{
			AccountID:     "123456789012",
			Region:        "us-east-1",
			ECRRepository: "ludus-server",
		},
		Container: config.ContainerConfig{ImageName: "ludus-server", Tag: "latest", ServerPort: 7777},
		Engine:    config.EngineConfig{Backend: "docker", Version: "5.7.3"},
		Game:      config.GameConfig{ProjectName: "MyGame"},
	}
	globals.SetGlobals(t, cfg, globals.WithNoLogs(true))
	stubAWSEnv(t)
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {},
		"aws":    {ExitCode: 1},
	})
	t.Chdir(t.TempDir())

	if err := runPush(pushCmd, nil); err == nil {
		t.Fatal("runPush() expected ECR failure")
	}
}

func TestRunPushResolverError(t *testing.T) {
	resetContainerFlags(t)
	cfg := &config.Config{
		AWS: config.AWSConfig{ECRRepository: "ludus-server"},
		Container: config.ContainerConfig{
			ImageName:  "ludus-server",
			Tag:        "latest",
			ServerPort: 7777,
		},
	}
	globals.SetGlobals(t, cfg, globals.WithNoLogs(true))
	stubAWSEnv(t)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {},
		"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
	})
	t.Chdir(t.TempDir())

	if err := runPush(pushCmd, nil); err == nil {
		t.Fatal("runPush() expected resolver error")
	}
}

func TestCheckBuildCacheCorruptCache(t *testing.T) {
	resetContainerFlags(t)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, ".ludus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ludus", "cache.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := checkBuildCache(cache.StageContainerBuild, "abc"); got {
		t.Error("checkBuildCache() with corrupt cache should return false")
	}
}
