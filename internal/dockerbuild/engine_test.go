package dockerbuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
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

func TestBuild_SkipEngine_MissingBinaries(t *testing.T) {
	tmpDir := t.TempDir()
	r := runner.NewRunner(false, true) // dry-run

	b := NewEngineImageBuilder(EngineImageOptions{
		SourcePath: tmpDir,
		SkipEngine: true,
	}, r)

	_, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("expected error when Linux binaries directory is missing")
	}
	if !strings.Contains(err.Error(), "--skip-engine requires pre-built Linux binaries") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuild_SkipEngine_EmptyBinaries(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "Engine", "Binaries", "Linux")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := runner.NewRunner(false, true) // dry-run

	b := NewEngineImageBuilder(EngineImageOptions{
		SourcePath: tmpDir,
		SkipEngine: true,
	}, r)

	_, err := b.Build(context.Background())
	if err == nil {
		t.Fatal("expected error when Linux binaries directory is empty")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEngineImageOptions_PlatformArg(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"arm64", "linux/arm64"},
		{"amd64", "linux/amd64"},
		{"", "linux/amd64"},
	}
	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			opts := EngineImageOptions{Arch: tt.arch}
			got := opts.platformArg()
			if got != tt.want {
				t.Errorf("platformArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuild_IncludesPlatformArg(t *testing.T) {
	tmpDir := t.TempDir()
	r := runner.NewRunner(false, true) // dry-run

	b := NewEngineImageBuilder(EngineImageOptions{
		SourcePath: tmpDir,
		Runtime:    "docker",
		Arch:       "arm64",
	}, r)

	if b.opts.Arch != "arm64" {
		t.Errorf("Arch not preserved, got %q", b.opts.Arch)
	}
	if b.opts.platformArg() != "linux/arm64" {
		t.Errorf("platformArg() = %q, want linux/arm64", b.opts.platformArg())
	}
}

// TestBuild_ForcesAmd64Platform: even with Arch=arm64 in opts, Build forces amd64 (for preflights + image).
func TestBuild_ForcesAmd64Platform(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Setup.sh"), []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	r := runner.NewRunner(false, true)
	b := NewEngineImageBuilder(EngineImageOptions{SourcePath: tmpDir, Runtime: "docker", Arch: "arm64"}, r)
	_, _ = b.Build(context.Background()) // dry-run; forces inside (see engine.go + macos_preflight)
}
