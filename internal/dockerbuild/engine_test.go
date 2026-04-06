package dockerbuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/runner"
)

func TestNewEngineImageBuilder(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name          string
		opts          EngineImageOptions
		wantImageName string
		wantImageTag  string
	}{
		{
			name:          "all defaults",
			opts:          EngineImageOptions{},
			wantImageName: "ludus-engine",
			wantImageTag:  "latest",
		},
		{
			name:          "version sets tag",
			opts:          EngineImageOptions{Version: "5.6.1"},
			wantImageName: "ludus-engine",
			wantImageTag:  "5.6.1",
		},
		{
			name:          "explicit tag overrides version",
			opts:          EngineImageOptions{Version: "5.6.1", ImageTag: "custom-tag"},
			wantImageName: "ludus-engine",
			wantImageTag:  "custom-tag",
		},
		{
			name:          "custom image name",
			opts:          EngineImageOptions{ImageName: "my-engine"},
			wantImageName: "my-engine",
			wantImageTag:  "latest",
		},
		{
			name:          "all custom",
			opts:          EngineImageOptions{ImageName: "my-engine", ImageTag: "v2", Version: "5.7.0"},
			wantImageName: "my-engine",
			wantImageTag:  "v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewEngineImageBuilder(tt.opts, r)
			if b.opts.ImageName != tt.wantImageName {
				t.Errorf("ImageName = %q, want %q", b.opts.ImageName, tt.wantImageName)
			}
			if b.opts.ImageTag != tt.wantImageTag {
				t.Errorf("ImageTag = %q, want %q", b.opts.ImageTag, tt.wantImageTag)
			}
		})
	}
}

func TestFullImageTag(t *testing.T) {
	r := runner.NewRunner(false, false)

	tests := []struct {
		name string
		opts EngineImageOptions
		want string
	}{
		{
			name: "defaults",
			opts: EngineImageOptions{},
			want: "ludus-engine:latest",
		},
		{
			name: "with version",
			opts: EngineImageOptions{Version: "5.6.1"},
			want: "ludus-engine:5.6.1",
		},
		{
			name: "custom name and tag",
			opts: EngineImageOptions{ImageName: "my-engine", ImageTag: "v2"},
			want: "my-engine:v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewEngineImageBuilder(tt.opts, r)
			got := b.FullImageTag()
			if got != tt.want {
				t.Errorf("FullImageTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewEngineImageBuilder_PreservesRunnerRef(t *testing.T) {
	r := runner.NewRunner(true, true)
	b := NewEngineImageBuilder(EngineImageOptions{}, r)
	if b.Runner != r {
		t.Error("NewEngineImageBuilder should store the provided Runner reference")
	}
}

func TestBuild_SkipCompile_MissingBinaries(t *testing.T) {
	tmpDir := t.TempDir()
	r := runner.NewRunner(false, true) // dry-run

	b := NewEngineImageBuilder(EngineImageOptions{
		SourcePath:  tmpDir,
		SkipCompile: true,
	}, r)

	_, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("expected error when Linux binaries directory is missing")
	}
	if !strings.Contains(err.Error(), "--skip-compile requires pre-built Linux binaries") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuild_SkipCompile_EmptyBinaries(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "Engine", "Binaries", "Linux")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := runner.NewRunner(false, true) // dry-run

	b := NewEngineImageBuilder(EngineImageOptions{
		SourcePath:  tmpDir,
		SkipCompile: true,
	}, r)

	_, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("expected error when Linux binaries directory is empty")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}
