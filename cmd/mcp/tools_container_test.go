package mcp

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHandleContainerBuildDryRun(t *testing.T) {
	tests := []struct {
		name  string
		input containerBuildInput
		cfg   config.Config
		want  string
	}{
		{
			name:  "input overrides",
			input: containerBuildInput{Tag: "candidate", Arch: runtime.GOARCH, Backend: "podman", NoCache: true, DryRun: true},
			cfg: config.Config{
				Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "project/Lyra.uproject", Arch: runtime.GOARCH},
				Container: config.ContainerConfig{ImageName: "server", Tag: "configured", ServerPort: 7777},
			},
			want: "server:candidate",
		},
		{
			name:  "configured tag",
			input: containerBuildInput{Backend: "docker", DryRun: true},
			cfg: config.Config{
				Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "project/Lyra.uproject"},
				Container: config.ContainerConfig{ImageName: "server", Tag: "stable", ServerPort: 7777},
			},
			want: "server:stable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withContainerTestConfig(t, &tt.cfg)
			result, _, err := handleContainerBuild(context.Background(), nil, tt.input)
			if err != nil {
				t.Fatalf("handleContainerBuild() error = %v", err)
			}
			if result.IsError {
				t.Fatalf("handleContainerBuild() returned error: %+v", result)
			}
			got := decodeContainerResult(t, result)
			if !got.Success || got.ImageTag != tt.want {
				t.Errorf("result = %+v, want success with image %q", got, tt.want)
			}
		})
	}
}

func TestHandleContainerBuildMissingDirectory(t *testing.T) {
	withContainerTestConfig(t, &config.Config{})

	result, _, err := handleContainerBuild(context.Background(), nil, containerBuildInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleContainerBuild() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("handleContainerBuild() should return an error result")
	}
	if got := decodeContainerResult(t, result); !strings.Contains(got.Error, "server build directory not specified") {
		t.Errorf("error = %q, want missing server build directory", got.Error)
	}
}

func withContainerTestConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	origCfg, origDryRun := globals.Cfg, globals.DryRun
	t.Cleanup(func() {
		globals.Cfg = origCfg
		globals.DryRun = origDryRun
	})
	t.Chdir(t.TempDir())
	globals.Cfg = cfg
	globals.DryRun = false
}

func decodeContainerResult(t *testing.T, result *mcpsdk.CallToolResult) containerResult {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected result content")
	}
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	var got containerResult
	if err := json.Unmarshal([]byte(text.Text), &got); err != nil {
		t.Fatalf("unmarshal container result: %v", err)
	}
	return got
}
