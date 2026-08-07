//go:build windows

package engine

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

// newQuietRunner returns a runner with output silenced so tests stay clean.
func newQuietRunner(dryRun bool) *runner.Runner {
	r := runner.NewRunner(false, dryRun)
	r.Stdout = io.Discard
	r.Stderr = io.Discard
	return r
}

func TestRunBatDryRun(t *testing.T) {
	b := NewBuilder(BuildOptions{SourcePath: t.TempDir()}, newQuietRunner(true))

	if err := b.runBat(context.Background(), "setup.bat", "-arg"); err != nil {
		t.Errorf("runBat(dry-run) error = %v, want nil", err)
	}
}

func TestRunBatExecutesStub(t *testing.T) {
	src := t.TempDir()
	stub := filepath.Join(src, "stub.bat")
	if err := os.WriteFile(stub, []byte("@echo off\r\nexit /b 0\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := NewBuilder(BuildOptions{SourcePath: src}, newQuietRunner(false))

	if err := b.runBat(context.Background(), stub); err != nil {
		t.Errorf("runBat(stub) error = %v, want nil", err)
	}
}

func TestRunBatExecutesFailingStub(t *testing.T) {
	src := t.TempDir()
	stub := filepath.Join(src, "stub.bat")
	if err := os.WriteFile(stub, []byte("@echo off\r\nexit /b 5\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := NewBuilder(BuildOptions{SourcePath: src}, newQuietRunner(false))

	if err := b.runBat(context.Background(), stub); err == nil {
		t.Error("runBat(failing stub) error = nil, want error")
	}
}

func TestRunBatFileMissing(t *testing.T) {
	b := NewBuilder(BuildOptions{SourcePath: t.TempDir()}, newQuietRunner(true))

	err := b.runBatFile(context.Background(), "Missing.bat")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("runBatFile() error = %v, want 'not found'", err)
	}
}

func TestSetupSuccess(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	b := NewBuilder(BuildOptions{SourcePath: root}, newQuietRunner(true))

	if err := b.Setup(context.Background()); err != nil {
		t.Errorf("Setup() error = %v, want nil", err)
	}
}

func TestSetupMissing(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithoutSetup())
	b := NewBuilder(BuildOptions{SourcePath: root}, newQuietRunner(true))

	err := b.Setup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Setup.bat not found") {
		t.Errorf("Setup() error = %v, want 'Setup.bat not found'", err)
	}
}

func TestGenerateProjectFilesSuccess(t *testing.T) {
	root := testsupport.FakeEngineTree(t)
	b := NewBuilder(BuildOptions{SourcePath: root}, newQuietRunner(true))

	if err := b.GenerateProjectFiles(context.Background()); err != nil {
		t.Errorf("GenerateProjectFiles() error = %v, want nil", err)
	}
}

func TestGenerateProjectFilesMissing(t *testing.T) {
	b := NewBuilder(BuildOptions{SourcePath: t.TempDir()}, newQuietRunner(true))

	err := b.GenerateProjectFiles(context.Background())
	if err == nil || !strings.Contains(err.Error(), "GenerateProjectFiles.bat not found") {
		t.Errorf("GenerateProjectFiles() error = %v, want 'not found'", err)
	}
}

func TestCompileSuccess(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	b := NewBuilder(BuildOptions{SourcePath: root}, newQuietRunner(true))

	if err := b.compile(context.Background(), 2); err != nil {
		t.Errorf("compile() error = %v, want nil", err)
	}
}

func TestCompileMissingBuildBat(t *testing.T) {
	b := NewBuilder(BuildOptions{SourcePath: t.TempDir()}, newQuietRunner(true))

	err := b.compile(context.Background(), 2)
	if err == nil || !strings.Contains(err.Error(), "Build.bat not found") {
		t.Errorf("compile() error = %v, want 'Build.bat not found'", err)
	}
}

func TestAutoDetectJobs(t *testing.T) {
	if got := autoDetectJobs(); got < 1 {
		t.Errorf("autoDetectJobs() = %d, want >= 1", got)
	}
}

// TestBuildCompileFailure covers the compile-failure branch of Build
// (builder.go:90-95) with a real (non-dry-run) engine tree: Setup and
// GenerateProjectFiles succeed, then the failing Build.bat stub makes the
// first compile target error, which propagates as the build error.
func TestBuildCompileFailure(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	buildBat := filepath.Join(root, "Engine", "Build", "BatchFiles", "Build.bat")
	if err := os.WriteFile(buildBat, []byte("@echo off\r\nexit /b 1\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := NewBuilder(BuildOptions{SourcePath: root}, newQuietRunner(false))

	result, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("Build() error = nil, want compile failure")
	}
	if result == nil || result.Success {
		t.Errorf("Build() result = %+v, want Success = false", result)
	}
	if !strings.Contains(err.Error(), "build ShaderCompileWorker failed") {
		t.Errorf("Build() error = %v, want ShaderCompileWorker failure", err)
	}
}

// TestBuildSuccessWindows covers the success tail of Build (builder.go:97-99):
// all steps run against the fake engine tree's exit-0 batch stubs and the
// result reports Success with a duration.
func TestBuildSuccessWindows(t *testing.T) {
	root := testsupport.FakeEngineTree(t, testsupport.WithVersion("5.7.3"))
	b := NewBuilder(BuildOptions{SourcePath: root}, newQuietRunner(false))

	result, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("Build() result = %+v, want Success = true", result)
	}
	if result.Duration <= 0 {
		t.Errorf("Build() Duration = %v, want > 0", result.Duration)
	}
}
