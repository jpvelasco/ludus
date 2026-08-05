package game

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// flowTestBuilder returns a Builder wired to a dry-run runner (hermetic: no
// UAT subprocess is spawned, only the would-be command is echoed).
func flowTestBuilder(opts BuildOptions) *Builder {
	return NewBuilder(opts, runner.NewRunner(false, true))
}

// flowBaseOptions returns BuildOptions referencing a fake engine tree and a
// fake project, sufficient to drive Build/BuildClient past LocateProject and
// resolveRunUAT.
func flowBaseOptions(t *testing.T) BuildOptions {
	t.Helper()
	engineRoot := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	projectPath := testsupport.FakeProject(t, "TestGame")
	return BuildOptions{
		EnginePath:    engineRoot,
		ProjectPath:   projectPath,
		ProjectName:   "TestGame",
		ServerTarget:  "TestGameServer",
		ClientTarget:  "TestGameClient",
		Platform:      "Linux",
		EngineVersion: "5.7.3",
	}
}

// makeDefaultEngineIniADir turns the project's Config/DefaultEngine.ini into a
// directory so os.ReadFile fails with a non-IsNotExist error.
func makeDefaultEngineIniADir(t *testing.T, projectPath string) {
	t.Helper()
	dir := filepath.Join(filepath.Dir(projectPath), "Config", "DefaultEngine.ini")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeFailingRunUAT overwrites both RunUAT scripts so the build step exits 1.
func writeFailingRunUAT(t *testing.T, engineRoot string) {
	t.Helper()
	batch := filepath.Join(engineRoot, "Engine", "Build", "BatchFiles")
	writeTestFile(t, filepath.Join(batch, "RunUAT.bat"), "@echo off\r\nexit /b 1\r\n")
	writeTestFile(t, filepath.Join(batch, "RunUAT.sh"), "#!/bin/sh\nexit 1\n")
}

// TestBuildSuccessDryRun covers the full Build pipeline: LocateProject →
// resolveRunUAT → prepareBuildEnvironment → resolveServerBuildArgs → setupDDC
// → runBuildStep, asserting the success result is fully populated.
func TestBuildSuccessDryRun(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	b := flowTestBuilder(opts)

	result, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !result.Success {
		t.Error("Build() Success = false, want true")
	}
	if result.Error != nil {
		t.Errorf("Build() Error = %v, want nil", result.Error)
	}
	wantOut := filepath.Join(filepath.Dir(opts.ProjectPath), "PackagedServer")
	if result.OutputDir != wantOut {
		t.Errorf("Build() OutputDir = %q, want %q", result.OutputDir, wantOut)
	}
	wantBin := filepath.Join(wantOut, "LinuxServer", "TestGameServer")
	if result.ServerBinary != wantBin {
		t.Errorf("Build() ServerBinary = %q, want %q", result.ServerBinary, wantBin)
	}
	if result.Duration <= 0 {
		t.Errorf("Build() Duration = %v, want > 0", result.Duration)
	}
}

// TestBuildArm64DefersDumpSyms covers the arm64 prepareBuildEnvironment branch
// (defer disableDumpSyms) on every host. APPDATA is sandboxed so the
// BuildConfiguration.xml workaround writes to a temp dir, and the file is
// asserted restored once the build completes.
func TestBuildArm64DefersDumpSyms(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	appdata := t.TempDir()
	t.Setenv("APPDATA", appdata)
	configPath := filepath.Join(appdata, "Unreal Engine", "UnrealBuildTool", "BuildConfiguration.xml")
	const original = "<BuildConfiguration>\n</BuildConfiguration>\n"
	writeTestFile(t, configPath, original)

	opts := flowBaseOptions(t)
	opts.Arch = "arm64"
	b := flowTestBuilder(opts)

	result, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("Build(arm64) error = %v", err)
	}
	if !result.Success {
		t.Error("Build(arm64) Success = false, want true")
	}
	wantBin := filepath.Join(filepath.Dir(opts.ProjectPath), "PackagedServer", "LinuxArm64Server", "TestGameServer")
	if result.ServerBinary != wantBin {
		t.Errorf("Build(arm64) ServerBinary = %q, want %q", result.ServerBinary, wantBin)
	}

	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading restored BuildConfiguration.xml: %v", err)
	}
	if string(restored) != original {
		t.Errorf("BuildConfiguration.xml not restored, got %q, want %q", restored, original)
	}
}

// TestBuildLocateProjectError covers the LocateProject failure branch.
func TestBuildLocateProjectError(t *testing.T) {
	opts := flowBaseOptions(t)
	opts.ProjectPath = filepath.Join(t.TempDir(), "missing.uproject")
	b := flowTestBuilder(opts)

	result, err := b.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "configured project path not found") {
		t.Fatalf("Build() error = %v, want project-not-found error", err)
	}
	if result.Error == nil {
		t.Error("Build() result.Error should be set on failure")
	}
}

// TestBuildResolveRunUATError covers resolveRunUAT failure in Build.
func TestBuildResolveRunUATError(t *testing.T) {
	opts := flowBaseOptions(t)
	opts.EnginePath = t.TempDir() // no Engine/Build/BatchFiles/RunUAT.*
	b := flowTestBuilder(opts)

	if _, err := b.Build(context.Background()); err == nil || !strings.Contains(err.Error(), "not found at") {
		t.Fatalf("Build() error = %v, want RunUAT-not-found error", err)
	}
}

// TestBuildPrepareEnvironmentError covers ensureDefaultServerTarget failing to
// read an unreadable DefaultEngine.ini (an IsNotExist error is graceful; any
// other read failure is surfaced).
func TestBuildPrepareEnvironmentError(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	makeDefaultEngineIniADir(t, opts.ProjectPath)
	b := flowTestBuilder(opts)

	if _, err := b.Build(context.Background()); err == nil || !strings.Contains(err.Error(), "setting default server target") {
		t.Fatalf("Build() error = %v, want DefaultServerTarget failure", err)
	}
}

// TestBuildSetupDDCError covers setupDDC rejecting an invalid DDC mode.
func TestBuildSetupDDCError(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	opts.DDCMode = "bogus"
	b := flowTestBuilder(opts)

	if _, err := b.Build(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported DDC mode") {
		t.Fatalf("Build() error = %v, want unsupported-DDC-mode error", err)
	}
}

// TestBuildRunStepError covers the RunUAT failure path via a real non-dry
// runner executing a failing script, then asserts diagnostics are applied.
func TestBuildRunStepError(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	writeFailingRunUAT(t, opts.EnginePath)

	b := NewBuilder(opts, runner.NewRunner(false, false))
	result, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("Build() error = nil, want RunUAT failure")
	}
	if !strings.Contains(err.Error(), "BuildCookRun") {
		t.Errorf("Build() error = %v, want BuildCookRun diagnostics", err)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "BuildCookRun") {
		t.Errorf("Build() result.Error = %v, want wrapped diagnostics", result.Error)
	}
}

// TestBuildClientSuccessDryRun covers BuildClient with the default Linux platform.
func TestBuildClientSuccessDryRun(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	b := flowTestBuilder(opts)

	result, err := b.BuildClient(context.Background())
	if err != nil {
		t.Fatalf("BuildClient() error = %v", err)
	}
	if !result.Success || result.Error != nil {
		t.Errorf("BuildClient() result = %+v, want success", result)
	}
	if result.Platform != "Linux" {
		t.Errorf("BuildClient() Platform = %q, want Linux", result.Platform)
	}
	wantOut := filepath.Join(filepath.Dir(opts.ProjectPath), "PackagedClient")
	if result.OutputDir != wantOut {
		t.Errorf("BuildClient() OutputDir = %q, want %q", result.OutputDir, wantOut)
	}
	if result.ClientBinary == "" || !strings.Contains(result.ClientBinary, "TestGameClient") {
		t.Errorf("BuildClient() ClientBinary = %q, want TestGameClient binary", result.ClientBinary)
	}
	if result.Duration <= 0 {
		t.Errorf("BuildClient() Duration = %v, want > 0", result.Duration)
	}
}

// TestBuildClientWin64Success covers BuildClient cross-compiling for Win64.
func TestBuildClientWin64Success(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	opts.ClientPlatform = "Win64"
	b := flowTestBuilder(opts)

	result, err := b.BuildClient(context.Background())
	if err != nil {
		t.Fatalf("BuildClient(Win64) error = %v", err)
	}
	if result.Platform != "Win64" {
		t.Errorf("BuildClient(Win64) Platform = %q, want Win64", result.Platform)
	}
	if !strings.HasSuffix(result.ClientBinary, ".exe") {
		t.Errorf("BuildClient(Win64) ClientBinary = %q, want .exe suffix", result.ClientBinary)
	}
}

// TestBuildClientPlatformError covers resolveClientPlatform rejecting an
// unsupported platform.
func TestBuildClientPlatformError(t *testing.T) {
	opts := flowBaseOptions(t)
	opts.ClientPlatform = "Android"
	b := flowTestBuilder(opts)

	if _, err := b.BuildClient(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported client platform") {
		t.Fatalf("BuildClient() error = %v, want unsupported-platform error", err)
	}
}

// TestBuildClientResolveRunUATError covers resolveRunUAT failure in BuildClient.
func TestBuildClientResolveRunUATError(t *testing.T) {
	opts := flowBaseOptions(t)
	opts.EnginePath = t.TempDir()
	b := flowTestBuilder(opts)

	if _, err := b.BuildClient(context.Background()); err == nil || !strings.Contains(err.Error(), "not found at") {
		t.Fatalf("BuildClient() error = %v, want RunUAT-not-found error", err)
	}
}

// TestBuildClientSetupDDCError covers setupDDC failure in BuildClient.
func TestBuildClientSetupDDCError(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	opts.DDCMode = "bogus"
	b := flowTestBuilder(opts)

	if _, err := b.BuildClient(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported DDC mode") {
		t.Fatalf("BuildClient() error = %v, want unsupported-DDC-mode error", err)
	}
}

// TestBuildClientRunStepError covers BuildClient's UAT failure diagnostics.
func TestBuildClientRunStepError(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	opts := flowBaseOptions(t)
	writeFailingRunUAT(t, opts.EnginePath)

	b := NewBuilder(opts, runner.NewRunner(false, false))
	if _, err := b.BuildClient(context.Background()); err == nil || !strings.Contains(err.Error(), "BuildCookRun (client") {
		t.Fatalf("BuildClient() error = %v, want client diagnostics", err)
	}
}

// TestExecRunUATDryRun exercises execRunUAT argument assembly for the current
// OS without spawning anything (dry-run runner).
func TestExecRunUATDryRun(t *testing.T) {
	engine := t.TempDir()
	b := flowTestBuilder(BuildOptions{EnginePath: engine})

	shell := "cmd"
	script := filepath.Join("Engine", "Build", "BatchFiles", "RunUAT.bat")
	if runtime.GOOS != "windows" {
		shell = "bash"
		script = filepath.Join("Engine", "Build", "BatchFiles", "RunUAT.sh")
	}
	if err := b.execRunUAT(context.Background(), shell, script, []string{"BuildCookRun"}); err != nil {
		t.Fatalf("execRunUAT() error = %v", err)
	}
}
