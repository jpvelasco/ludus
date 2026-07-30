package pipeline

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/wsl"
)

func TestResolveWSL2GameDDCPathFromState(t *testing.T) {
	w := &wsl.WSL2{Distro: "Ubuntu-24.04"}

	engineState := &state.WSL2EngineState{
		DDCPath: "/home/user/ludus/ddc",
	}

	got := resolveWSL2GameDDCPath(engineState, "local", "C:/ludus/ddc", w)
	if got != "/home/user/ludus/ddc" {
		t.Errorf("resolveWSL2GameDDCPath() = %q, want %q", got, "/home/user/ludus/ddc")
	}
}

func TestResolveWSL2GameDDCPathFallbackVirtiofs(t *testing.T) {
	w := &wsl.WSL2{Distro: "Ubuntu-24.04"}

	engineState := &state.WSL2EngineState{
		DDCPath: "",
	}

	got := resolveWSL2GameDDCPath(engineState, "local", `C:\ludus\ddc`, w)
	if got == "" {
		t.Errorf("resolveWSL2GameDDCPath() returned empty string, want path")
	}
}

func TestResolveWSL2GameDDCPathModeNone(t *testing.T) {
	w := &wsl.WSL2{Distro: "Ubuntu-24.04"}

	engineState := &state.WSL2EngineState{
		DDCPath: "",
	}

	got := resolveWSL2GameDDCPath(engineState, "none", `C:\ludus\ddc`, w)
	if got != "" {
		t.Errorf("resolveWSL2GameDDCPath() = %q, want empty for mode=none", got)
	}
}

func TestWSL2Fallback(t *testing.T) {
	err := wsl2Fallback(nil)
	if err == nil {
		t.Fatal("wsl2Fallback() expected error, got nil")
	}
}
