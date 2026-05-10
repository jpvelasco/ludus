package container

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		want        string
	}{
		{"explicit", "MyGame", "MyGame"},
		{"empty defaults to Lyra", "", "Lyra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{ProjectName: tt.projectName}, nil)
			got := b.resolveProjectName()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveServerTarget(t *testing.T) {
	tests := []struct {
		name         string
		serverTarget string
		projectName  string
		want         string
	}{
		{"explicit", "CustomServer", "", "CustomServer"},
		{"default from project", "", "MyGame", "MyGameServer"},
		{"default Lyra", "", "", "LyraServer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{
				ServerTarget: tt.serverTarget,
				ProjectName:  tt.projectName,
			}, nil)
			got := b.resolveServerTarget()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveArch(t *testing.T) {
	tests := []struct {
		name string
		arch string
		want string
	}{
		{"empty defaults to amd64", "", "amd64"},
		{"amd64", "amd64", "amd64"},
		{"arm64", "arm64", "arm64"},
		{"aarch64 normalized", "aarch64", "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(BuildOptions{Arch: tt.arch}, nil)
			got := b.resolveArch()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveServerBinaryName(t *testing.T) {
	t.Run("no build dir", func(t *testing.T) {
		b := NewBuilder(BuildOptions{
			ProjectName:  "Lyra",
			ServerTarget: "LyraServer",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer" {
			t.Errorf("got %q, want %q", got, "LyraServer")
		}
	})

	t.Run("development build", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "Lyra", "Binaries", "Linux")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Development build: bare target name
		if err := os.WriteFile(filepath.Join(binDir, "LyraServer"), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}

		b := NewBuilder(BuildOptions{
			ServerBuildDir: tmpDir,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer" {
			t.Errorf("got %q, want %q", got, "LyraServer")
		}
	})

	t.Run("shipping build", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "Lyra", "Binaries", "Linux")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Shipping build: Target-Platform-Config pattern
		if err := os.WriteFile(filepath.Join(binDir, "LyraServer-Linux-Shipping"), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}

		b := NewBuilder(BuildOptions{
			ServerBuildDir: tmpDir,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer-Linux-Shipping" {
			t.Errorf("got %q, want %q", got, "LyraServer-Linux-Shipping")
		}
	})

	t.Run("arm64 shipping build", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "Lyra", "Binaries", "LinuxArm64")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "LyraServer-LinuxArm64-Shipping"), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}

		b := NewBuilder(BuildOptions{
			ServerBuildDir: tmpDir,
			ProjectName:    "Lyra",
			ServerTarget:   "LyraServer",
			Arch:           "arm64",
		}, nil)
		got := b.resolveServerBinaryName()
		if got != "LyraServer-LinuxArm64-Shipping" {
			t.Errorf("got %q, want %q", got, "LyraServer-LinuxArm64-Shipping")
		}
	})
}
