package wsl

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestBuildGameValidationError(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	_, err := BuildGame(context.Background(), w, GameOptions{})
	if err == nil || !strings.Contains(err.Error(), "engine path is empty") {
		t.Errorf("BuildGame() error = %v, want 'engine path is empty'", err)
	}
}

func TestBuildGameValidationLocalDDCPathError(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	_, err := BuildGame(context.Background(), w, GameOptions{
		EnginePath: "/opt/ue/5.7",
		DDCMode:    ddc.ModeLocal,
	})
	if err == nil || !strings.Contains(err.Error(), "DDC path is empty") {
		t.Errorf("BuildGame() error = %v, want 'DDC path is empty'", err)
	}
}

func TestBuildGameSuccess(t *testing.T) {
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, true)}

	result, err := BuildGame(context.Background(), w, GameOptions{
		EnginePath:  "/opt/ue/5.7",
		ProjectPath: `C:\proj\MyGame.uproject`,
		ProjectName: "MyGame",
		Platform:    "Linux",
		Arch:        "amd64",
		DDCMode:     ddc.ModeNone,
		OutputDir:   `C:\out`,
	})
	if err != nil {
		t.Fatalf("BuildGame() error = %v", err)
	}
	if !result.Success {
		t.Error("BuildGame() result.Success = false, want true")
	}
	if result.OutputDir != "/mnt/c/out" {
		t.Errorf("BuildGame() OutputDir = %q, want /mnt/c/out", result.OutputDir)
	}
}

func TestBuildGameRuntimeDepsError(t *testing.T) {
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	w := &WSL2{Distro: "Ubuntu", Runner: newTestRunner(t, false)}

	_, err := BuildGame(context.Background(), w, GameOptions{
		EnginePath:  "/opt/ue/5.7",
		ProjectPath: `C:\proj\MyGame.uproject`,
		DDCMode:     ddc.ModeNone,
	})
	if err == nil || !strings.Contains(err.Error(), "ensuring runtime dependencies") {
		t.Errorf("BuildGame() error = %v, want 'ensuring runtime dependencies'", err)
	}
}
