package status

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestContainerEngineStatus(t *testing.T) {
	tests := []struct {
		name, backend, prebuilt, recorded, want, detail string
		exit                                            int
	}{
		{"docker image exists", "docker", "", "", "ok", "example-engine:5.8.4", 0},
		{"podman image exists", "podman", "", "", "ok", "example-engine:5.8.4", 0},
		{"missing image", "docker", "", "", "fail", "example-engine:5.8.4", 1},
		{"configured prebuilt", "docker", "registry.example/engine:stable", "", "ok", "configured prebuilt image: registry.example/engine:stable", 1},
		{"recorded image", "podman", "", "recorded-engine:5.7.4", "ok", "recorded-engine:5.7.4", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			testsupport.FakeTool(t, tt.backend, testsupport.ToolBehavior{ExitCode: tt.exit, Stderr: "image unavailable"})
			cfg := &config.Config{Engine: config.EngineConfig{
				Backend: tt.backend, DockerImage: tt.prebuilt, DockerImageName: "example-engine", Version: "5.8.4",
			}}
			if tt.recorded != "" {
				if err := state.UpdateEngineImage(&state.EngineImageState{ImageTag: tt.recorded}); err != nil {
					t.Fatal(err)
				}
			}
			stages := CheckAll(context.Background(), cfg, nil)
			got := stages[1]
			if got.Name != "Engine Build" || got.Status != tt.want || !strings.Contains(got.Detail, tt.detail) {
				t.Fatalf("engine stage = %+v, want %s with %q", got, tt.want, tt.detail)
			}
		})
	}
}

func TestEngineStatusUnavailableRuntime(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	cfg := &config.Config{Engine: config.EngineConfig{Backend: "docker", DockerImageName: "example"}}
	got := CheckEngineBuild(context.Background(), cfg)
	if got.Status != "unknown" || !strings.Contains(got.Detail, "docker not available") {
		t.Fatalf("unexpected runtime status: %+v", got)
	}
}

func TestNativeEngineStatusKeepsBinaryCheck(t *testing.T) {
	for _, backend := range []string{"", "native", "wsl2"} {
		t.Run(backend, func(t *testing.T) {
			cfg := &config.Config{Engine: config.EngineConfig{Backend: backend, SourcePath: t.TempDir()}}
			got := CheckEngineBuild(context.Background(), cfg)
			if got.Status != "fail" || !strings.Contains(got.Detail, "UnrealEditor") {
				t.Fatalf("expected native binary check: %+v", got)
			}
		})
	}
}
