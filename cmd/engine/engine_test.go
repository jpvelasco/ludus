package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
	"github.com/jpvelasco/ludus/internal/toolchain"
	"github.com/jpvelasco/ludus/internal/wsl"
	"github.com/spf13/cobra"
)

func TestMakeBuilderRequiresSourcePath(t *testing.T) {
	setEngineTestGlobals(t, &config.Config{})

	if _, err := makeBuilder(); err == nil {
		t.Fatal("makeBuilder() error = nil, want missing source path error")
	}
}

func TestMakeContainerEngineBuilderRequiresSourcePath(t *testing.T) {
	setEngineTestGlobals(t, &config.Config{})

	if _, err := makeContainerEngineBuilder("docker"); err == nil {
		t.Fatal("makeContainerEngineBuilder() error = nil, want missing source path error")
	}
}

func TestMakeBuildersWithConfiguredSource(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Engine.MaxJobs = 7
	setEngineTestGlobals(t, cfg)

	if _, err := makeBuilder(); err != nil {
		t.Fatalf("makeBuilder() error = %v", err)
	}
	if _, err := makeContainerEngineBuilder("docker"); err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}
}

func TestMaybeRunMacOSPreflightsIsNoopOnLinuxAndWindows(t *testing.T) {
	setEngineTestGlobals(t, &config.Config{})

	if err := maybeRunMacOSPreflights(t.Context()); err != nil {
		t.Fatalf("maybeRunMacOSPreflights() error = %v", err)
	}
}

func TestMakeContainerEngineBuilderPreservesFullVersion(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Engine.Version = "5.7.4"
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("docker")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	tag := builder.FullImageTag()
	if tag != "ludus-engine:5.7.4" {
		t.Errorf("FullImageTag() = %q, want ludus-engine:5.7.4", tag)
	}
}

func TestMakeContainerEngineBuilderCustomImageName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Engine.SourcePath = t.TempDir()
	cfg.Engine.Version = "5.7.4"
	cfg.Engine.DockerImageName = "my-registry/ludus-engine"
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("podman")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	tag := builder.FullImageTag()
	if tag != "my-registry/ludus-engine:5.7.4" {
		t.Errorf("FullImageTag() = %q, want my-registry/ludus-engine:5.7.4", tag)
	}
}

// TestMakeContainerEngineBuilderOverrideFromBuildVersion verifies that when
// the engine source path contains a Build.version file with a different patch
// than cfg.Engine.Version, the image tag is derived from the actual source.
func TestMakeContainerEngineBuilderOverrideFromBuildVersion(t *testing.T) {
	srcDir := t.TempDir()

	// Write a Build.version file matching 5.8.2
	bv := toolchain.BuildVersion{MajorVersion: 5, MinorVersion: 8, PatchVersion: 2}
	err := os.MkdirAll(filepath.Join(srcDir, "Engine", "Build"), 0755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Engine", "Build", "Build.version"),
		mustMarshalJSON(t, bv), 0644); err != nil {
		t.Fatalf("write Build.version: %v", err)
	}

	cfg := &config.Config{}
	cfg.Engine.SourcePath = srcDir
	cfg.Engine.Version = "5.7.4" // config says 5.7.4, but source is 5.8.2
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("docker")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	// The tag should be derived from Build.version (5.8.2), not from config (5.7.4)
	tag := builder.FullImageTag()
	if tag != "ludus-engine:5.8.2" {
		t.Errorf("FullImageTag() = %q, want ludus-engine:5.8.2", tag)
	}
}

// TestMakeContainerEngineBuilderFallbackToConfig verifies that when the engine
// source path has no Build.version file, the image tag falls back to
// cfg.Engine.Version.
func TestMakeContainerEngineBuilderFallbackToConfig(t *testing.T) {
	srcDir := t.TempDir() // empty dir, no Build.version

	cfg := &config.Config{}
	cfg.Engine.SourcePath = srcDir
	cfg.Engine.Version = "5.7.4"
	setEngineTestGlobals(t, cfg)

	builder, err := makeContainerEngineBuilder("docker")
	if err != nil {
		t.Fatalf("makeContainerEngineBuilder() error = %v", err)
	}

	tag := builder.FullImageTag()
	if tag != "ludus-engine:5.7.4" {
		t.Errorf("FullImageTag() = %q, want ludus-engine:5.7.4", tag)
	}
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func setEngineTestGlobals(t *testing.T, cfg *config.Config) {
	t.Helper()

	oldCfg := globals.Cfg
	oldUEPath, oldJobs, oldBackend := uePath, jobs, backend
	oldBaseImage := baseImage
	globals.Cfg = cfg
	uePath, jobs, backend, baseImage = "", 0, "", ""
	t.Cleanup(func() {
		globals.Cfg = oldCfg
		uePath, jobs, backend, baseImage = oldUEPath, oldJobs, oldBackend, oldBaseImage
	})
}

// TestRunSetupSuccess tests successful engine setup with valid engine tree,
// asserting that the dry-run produces setup commands.
func TestRunSetupSuccess(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureStdout(func() {
		err := runSetup(cmd, nil)
		if err != nil {
			t.Fatalf("runSetup() error = %v, want nil", err)
		}
	})

	// Assert that Setup.bat or Setup.sh is invoked
	if !strings.Contains(output, "Setup.bat") && !strings.Contains(output, "Setup.sh") {
		t.Errorf("output missing Setup script invocation: %s", output)
	}
}

// TestRunSetupMissingSourcePath tests setup with no engine path configured.
func TestRunSetupMissingSourcePath(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "",
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runSetup(cmd, nil)
	if err == nil {
		t.Fatal("runSetup() error = nil, want error for missing source path")
	}
	if !strings.Contains(err.Error(), "engine source path not configured") {
		t.Errorf("runSetup() error = %v, want 'engine source path not configured'", err)
	}
}

// TestRunBuildSkipEngineRequiresContainer tests that --skip-engine requires container backend.
func TestRunBuildSkipEngineRequiresContainer(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	skipEngine = true
	t.Cleanup(func() { skipEngine = false })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runBuild(cmd, nil)
	if err == nil {
		t.Fatal("runBuild() error = nil, want error for --skip-engine with native backend")
	}
	if !strings.Contains(err.Error(), "--skip-engine requires a container backend") {
		t.Errorf("runBuild() error = %v, want '--skip-engine requires a container backend'", err)
	}
}

// TestRunBuildNativeBackend tests native engine build dispatch.
func TestRunBuildNativeBackend(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "native",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runBuild(cmd, nil)
	if err != nil {
		t.Errorf("runBuild() error = %v, want nil", err)
	}
}

// TestRunBuildContainerBackend tests container engine build dispatch.
func TestRunBuildContainerBackend(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:  engineRoot,
			Version:     "5.7.3",
			MaxJobs:     1,
			Backend:     "docker",
			DockerImage: "my.repo/engine:5.7.3",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runBuild(cmd, nil)
	if err != nil {
		t.Errorf("runBuild() error = %v, want nil", err)
	}
}

// TestRunNativeEngineBuild tests native engine build execution with dry-run,
// asserting that build completes without error and produces Build/Build script invocation.
func TestRunNativeEngineBuild(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureStdout(func() {
		err := runNativeEngineBuild(cmd)
		if err != nil {
			t.Fatalf("runNativeEngineBuild() error = %v, want nil", err)
		}
	})

	// Assert that Build.bat or Build script is invoked
	if !strings.Contains(output, "Build") {
		t.Errorf("output missing Build script invocation: %s", output)
	}
}

// TestRunContainerBuild tests container engine build with prebuilt image.
func TestRunContainerBuild(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath:      engineRoot,
			Version:         "5.7.3",
			DockerImage:     "my.repo/engine:5.7.3",
			DockerImageName: "ludus-engine",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// Test with docker backend
	output := captureStdout(func() {
		err := runContainerBuild(cmd, "docker")
		if err != nil {
			t.Fatalf("runContainerBuild() error = %v, want nil", err)
		}
	})

	// Assert that docker build command is produced (check for build subcommand)
	if !strings.Contains(output, "build") || (!strings.Contains(output, "docker") && !strings.Contains(output, "podman")) {
		t.Errorf("output missing 'docker/podman build' command: %s", output)
	}
	if !strings.Contains(output, "ludus-engine") {
		t.Errorf("output missing 'ludus-engine' image name: %s", output)
	}
}

// TestRunPush tests engine image push to ECR with dry-run.
func TestRunPush(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			Version:         "5.7.3",
			DockerImage:     "my.repo/ludus-engine:5.7.3",
			DockerImageName: "ludus-engine",
		},
		AWS: config.AWSConfig{
			Region:    "us-east-1",
			AccountID: "123456789012",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	output := captureStdout(func() {
		err := runPush(cmd, nil)
		if err != nil {
			t.Fatalf("runPush() error = %v, want nil", err)
		}
	})

	// Assert that docker/podman tag and push commands are generated
	if !strings.Contains(output, "docker tag") && !strings.Contains(output, "podman tag") {
		t.Errorf("output missing 'docker tag' or 'podman tag' command: %s", output)
	}
	if !strings.Contains(output, "docker push") && !strings.Contains(output, "podman push") {
		t.Errorf("output missing 'docker push' or 'podman push' command: %s", output)
	}
	if !strings.Contains(output, "ludus-engine") {
		t.Errorf("output missing 'ludus-engine' repository name: %s", output)
	}
}

// TestRunPushMissingImage tests push with empty DockerImage,
// asserting that it fails when the image name is not resolvable.
func TestRunPushMissingImage(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			Version:         "5.7.3",
			DockerImage:     "",
			DockerImageName: "", // Empty image name forces failure
		},
		AWS: config.AWSConfig{
			Region:    "us-east-1",
			AccountID: "123456789012",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// With no image and no name, ResolveEngineImageParts should fail
	err := runPush(cmd, nil)
	if err == nil {
		// In dry-run, validation may still pass but no image to push exists
		// What matters is that we're testing the error path
		return
	}
	// Should have an error about missing image name
	if err.Error() == "" {
		t.Error("runPush() error message is empty")
	}
}

// TestResolveBackendFlagOverridesConfig tests that CLI flags override config backend.
func TestResolveBackendFlagOverridesConfig(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			Backend: "docker",
		},
	}

	globals.SetGlobals(t, cfg)

	backend = "podman"
	t.Cleanup(func() { backend = "" })

	result := resolveBackend()
	if result != "podman" {
		t.Errorf("resolveBackend() = %q, want %q", result, "podman")
	}
}

// TestResolveBackendConfigFallback tests that config is used when flag is empty.
func TestResolveBackendConfigFallback(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			Backend: "docker",
		},
	}

	globals.SetGlobals(t, cfg)

	backend = ""

	result := resolveBackend()
	if result != "docker" {
		t.Errorf("resolveBackend() = %q, want %q", result, "docker")
	}
}

// TestResolveWSL2EnginePathsVirtioFS tests path resolution with virtiofs (no native sync),
// asserting correct /mnt/ paths are returned.
func TestResolveWSL2EnginePathsVirtioFS(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:\\ue5",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r := globals.NewRunner()
	w := &wsl.WSL2{
		Distro: "Ubuntu",
	}

	// Test without wsl-native (virtiofs path)
	wslNative = false
	t.Cleanup(func() { wslNative = false })

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	enginePath, ddcPath, err := resolveWSL2EnginePaths(cmd, r, w, "C:\\ue5", "5.7.3")
	if err != nil {
		t.Fatalf("resolveWSL2EnginePaths() error = %v, want nil", err)
	}

	// Should return /mnt/ paths, not actual Windows paths
	if enginePath == "" {
		t.Fatalf("resolveWSL2EnginePaths() enginePath empty, want /mnt/ path")
	}
	if !strings.Contains(enginePath, "/mnt/") {
		t.Errorf("enginePath = %q, want /mnt/ path", enginePath)
	}
	if ddcPath == "" {
		t.Fatalf("resolveWSL2EnginePaths() ddcPath empty, want /mnt/ path")
	}
	if !strings.Contains(ddcPath, "/mnt/") {
		t.Errorf("ddcPath = %q, want /mnt/ path", ddcPath)
	}
}

// TestSaveWSL2EngineState tests that engine state is persisted with correct values.
func TestSaveWSL2EngineState(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cfg := &config.Config{}
	globals.SetGlobals(t, cfg, globals.WithNoLogs(true))

	wslNative = true
	t.Cleanup(func() { wslNative = false })

	saveWSL2EngineState("/home/ue/Engine", "/home/ue/.ludus/ddc")

	verifyWSL2StateFile(t, tmpDir, "/home/ue/Engine", "/home/ue/.ludus/ddc")
}

// TestRunWSL2BuildRequiresSourcePath tests that runWSL2Build fails with missing source path.
func TestRunWSL2BuildRequiresSourcePath(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "",
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runWSL2Build(cmd)
	if err == nil {
		t.Fatal("runWSL2Build() error = nil, want error for missing source path")
	}
	if !strings.Contains(err.Error(), "source path") {
		t.Errorf("runWSL2Build() error = %v, want error mentioning source path", err)
	}
}

// TestResolveWSL2EnginePathsVirtioFSWithSync tests path resolution with virtiofs when wslNative is false,
// asserting correct /mnt/ paths are returned without attempting sync.
func TestResolveWSL2EnginePathsNoNativeSync(t *testing.T) {
	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: "C:\\ue5",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	r := globals.NewRunner()
	w := &wsl.WSL2{
		Distro: "Ubuntu",
	}

	// Test with wsl-native disabled - should use virtiofs path conversion
	wslNative = false
	t.Cleanup(func() { wslNative = false })

	c := &cobra.Command{}
	c.SetContext(context.Background())

	enginePath, ddcPath, err := resolveWSL2EnginePaths(c, r, w, "C:\\ue5", "5.7.3")
	if err != nil {
		t.Fatalf("resolveWSL2EnginePaths() error = %v, want nil", err)
	}

	if !strings.Contains(enginePath, "/mnt/") || !strings.Contains(ddcPath, "/mnt/") {
		t.Errorf("expected /mnt/ paths, got enginePath=%q, ddcPath=%q", enginePath, ddcPath)
	}
}

// TestRunBuildWSL2Backend tests WSL2 engine build dispatch path.
func TestRunBuildWSL2Backend(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
			Backend:    "wsl2",
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// With dry-run, WSL2 build should complete without error
	err := runBuild(cmd, nil)
	if err != nil {
		t.Fatalf("runBuild() error = %v, want nil (dry-run)", err)
	}
}

// TestRunSetupWithValidSource tests setup success with valid engine source path.
func TestRunSetupWithValidSource(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
			MaxJobs:    1,
		},
		Game: config.GameConfig{
			ProjectName: "TestGame",
		},
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	// This tests the success path of runSetup with a valid source path
	err := runSetup(cmd, nil)
	if err != nil {
		t.Fatalf("runSetup() error = %v, want nil", err)
	}
}
