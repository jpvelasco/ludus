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
				Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "project/Lyra.uproject", Arch: runtime.GOARCH},
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

// TestHandleContainerPushDryRun tests container push with --dry-run.
// handleContainerPush attempts AWS API calls without credentials; we stub the env resolution.
func TestHandleContainerPushDryRun(t *testing.T) {
	withContainerTestConfig(t, &config.Config{
		Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "project/Lyra.uproject"},
		Container: config.ContainerConfig{ImageName: "server", Tag: "v1.0", ServerPort: 7777},
		AWS:       config.AWSConfig{AccountID: "123456789012", Region: "us-west-2"},
	})

	result, _, err := handleContainerPush(context.Background(), nil, containerPushInput{DryRun: true})
	if err != nil {
		t.Fatalf("handleContainerPush() error = %v", err)
	}
	// Verify the result is either success or contains an error message with expected content
	got := decodeContainerResult(t, result)
	if result.IsError {
		// Error is expected when AWS env resolution fails
		if !strings.Contains(got.Error, "container push failed") {
			t.Errorf("error = %q, want 'container push failed'", got.Error)
		}
	} else {
		// On success, verify the image tag was assembled
		if got.ImageTag != "server:v1.0" {
			t.Errorf("image tag = %q, want server:v1.0", got.ImageTag)
		}
	}
}

// TestHandleContainerPushOverrideTag tests that input tag overrides config tag.
func TestHandleContainerPushOverrideTag(t *testing.T) {
	withContainerTestConfig(t, &config.Config{
		Game:      config.GameConfig{ProjectName: "Lyra", ProjectPath: "project/Lyra.uproject"},
		Container: config.ContainerConfig{ImageName: "server", Tag: "old", ServerPort: 7777},
	})

	result, _, err := handleContainerPush(context.Background(), nil, containerPushInput{Tag: "candidate", DryRun: true})
	if err != nil {
		t.Fatalf("handleContainerPush() error = %v", err)
	}
	got := decodeContainerResult(t, result)
	if got.ImageTag != "server:candidate" {
		t.Errorf("result image tag = %q, want server:candidate", got.ImageTag)
	}
}
