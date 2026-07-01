package game

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	gameBuilder "github.com/jpvelasco/ludus/internal/game"
	"github.com/jpvelasco/ludus/internal/state"
)

func TestPrintBuildConfigGuidance(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want string
	}{
		{name: "shipping", cfg: "Shipping", want: "Shipping (optimized"},
		{name: "development", cfg: "Development", want: "Development (debug symbols"},
		{name: "empty", cfg: "", want: ""},
		{name: "custom", cfg: "Test", want: "Build config: Test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureGameStdout(t, func() { printBuildConfigGuidance(tt.cfg) })
			if tt.want == "" {
				if got != "" {
					t.Errorf("output = %q, want no output", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("output = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestNextAfterServerBuild(t *testing.T) {
	original := globals.Cfg
	t.Cleanup(func() { globals.Cfg = original })
	tests := []struct {
		target string
		want   string
	}{
		{target: "gamelift", want: "ludus container build"},
		{target: "STACK", want: "ludus container build"},
		{target: "ec2", want: "ludus deploy ec2"},
		{target: "anywhere", want: "ludus deploy anywhere"},
		{target: "binary", want: "ludus deploy binary"},
		{target: "", want: "ludus container build  (or: ludus deploy <target>)"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			globals.Cfg = &config.Config{}
			globals.Cfg.Deploy.Target = tt.target
			if got := nextAfterServerBuild(); got != tt.want {
				t.Errorf("nextAfterServerBuild() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveGameBackendAndArch(t *testing.T) {
	originalCfg, originalBackend, originalArch := globals.Cfg, backend, archFlag
	t.Cleanup(func() {
		globals.Cfg, backend, archFlag = originalCfg, originalBackend, originalArch
	})
	globals.Cfg = &config.Config{}
	globals.Cfg.Engine.Backend = "docker"
	globals.Cfg.Game.Arch = "amd64"
	backend, archFlag = "", ""
	if got := resolveBackend(); got != "docker" {
		t.Errorf("configured backend = %q, want docker", got)
	}
	if got := resolveArch(); got != "amd64" {
		t.Errorf("configured arch = %q, want amd64", got)
	}
	backend, archFlag = "podman", "aarch64"
	if got := resolveBackend(); got != "podman" {
		t.Errorf("flag backend = %q, want podman", got)
	}
	if got := resolveArch(); got != "arm64" {
		t.Errorf("flag arch = %q, want arm64", got)
	}
}

func TestSaveClientState(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })
	saveClientState(&gameBuilder.ClientBuildResult{
		ClientBinary: "build/MyGame",
		OutputDir:    "build",
		Platform:     "Linux",
	})
	got, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}
	if got.Client == nil {
		t.Fatal("client state is nil")
	}
	fields := map[string]struct {
		got  string
		want string
	}{
		"binary":   {got: got.Client.BinaryPath, want: "build/MyGame"},
		"output":   {got: got.Client.OutputDir, want: "build"},
		"platform": {got: got.Client.Platform, want: "Linux"},
	}
	for name, field := range fields {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", name, field.got, field.want)
		}
	}
	if got.Client.BuiltAt == "" {
		t.Error("BuiltAt is empty")
	}
}

func captureGameStdout(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous })
	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	os.Stdout = previous
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(output)
}
