package wsl

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestBuildEngineVirtioFS(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	result, err := BuildEngine(context.Background(), w, EngineOptions{
		SourcePath: `C:\ue5`,
		MaxJobs:    2,
		WSLNative:  false,
		Version:    "5.7",
	})
	if err != nil {
		t.Fatalf("BuildEngine() error = %v", err)
	}
	if !result.Success {
		t.Error("BuildEngine() result.Success = false, want true")
	}
	if result.EnginePath != "/mnt/c/ue5" {
		t.Errorf("BuildEngine() EnginePath = %q, want %q", result.EnginePath, "/mnt/c/ue5")
	}
}

func TestBuildEngineNativeExpandsHome(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	result, err := BuildEngine(context.Background(), w, EngineOptions{
		SourcePath: `C:\ue5`,
		WSLNative:  true,
		Version:    "5.7",
	})
	if err != nil {
		t.Fatalf("BuildEngine() error = %v", err)
	}
	if !strings.Contains(result.EnginePath, "ludus/engine/5.7") {
		t.Errorf("BuildEngine() EnginePath = %q, want native engine path", result.EnginePath)
	}
}

func TestBuildEngineEmptyPath(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	_, err := BuildEngine(context.Background(), w, EngineOptions{
		SourcePath: "",
		WSLNative:  false,
	})
	if err == nil || !strings.Contains(err.Error(), "engine path is empty") {
		t.Errorf("BuildEngine() error = %v, want 'engine path is empty'", err)
	}
}

func TestBuildEngineResolvePathError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	_, err := BuildEngine(context.Background(), w, EngineOptions{
		SourcePath: `C:\ue5`,
		WSLNative:  true,
		Version:    "5.7",
	})
	if err == nil || !strings.Contains(err.Error(), "resolving engine path") {
		t.Errorf("BuildEngine() error = %v, want 'resolving engine path'", err)
	}
}

func TestBuildEngineEnsureDepsError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	_, err := BuildEngine(context.Background(), w, EngineOptions{
		SourcePath: `C:\ue5`,
		WSLNative:  false,
	})
	if err == nil || !strings.Contains(err.Error(), "ensuring build dependencies") {
		t.Errorf("BuildEngine() error = %v, want 'ensuring build dependencies'", err)
	}
}

func TestRunEngineStepsSuccess(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	if err := runEngineSteps(context.Background(), w, "/mnt/c/ue5", 4); err != nil {
		t.Errorf("runEngineSteps() error = %v, want nil", err)
	}
}

func TestRunEngineStepsSetupFailure(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	err := runEngineSteps(context.Background(), w, "/mnt/c/ue5", 4)
	if err == nil || !strings.Contains(err.Error(), "Setup.sh failed") {
		t.Errorf("runEngineSteps() error = %v, want 'Setup.sh failed'", err)
	}
}
