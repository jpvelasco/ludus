package dockerbuild

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestIsWSL2Backend(t *testing.T) {
	tests := []struct {
		backend string
		want    bool
	}{
		{backend: BackendWSL2, want: true},
		{backend: BackendDocker},
		{backend: BackendNative},
		{backend: ""},
		{backend: "WSL2"},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			if got := IsWSL2Backend(tt.backend); got != tt.want {
				t.Errorf("IsWSL2Backend(%q) = %v, want %v", tt.backend, got, tt.want)
			}
		})
	}
}

func TestContainerCLI_UsesPath(t *testing.T) {
	path := t.TempDir()
	executable := writeTestExecutable(t, path, BackendDocker)
	t.Setenv("PATH", path)
	if got := ContainerCLI(BackendDocker); got != executable {
		t.Errorf("ContainerCLI(docker) = %q, want %q", got, executable)
	}
}

func TestContainerCLI_PodmanFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only fallback")
	}
	// podman not in PATH; the fallback list resolves it.
	fakePath := testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{})
	original := podmanWindowsPaths
	t.Cleanup(func() { podmanWindowsPaths = original })
	podmanWindowsPaths = []string{fakePath}

	t.Setenv("PATH", t.TempDir())
	got := ContainerCLI(BackendPodman)
	if got != fakePath {
		t.Errorf("ContainerCLI(podman) = %q, want fallback %q", got, fakePath)
	}
}

func TestContainerCLI_PodmanNoFallback(t *testing.T) {
	// Neither PATH nor the fallback list finds podman; bare name is returned.
	original := podmanWindowsPaths
	t.Cleanup(func() { podmanWindowsPaths = original })
	podmanWindowsPaths = []string{filepath.Join(t.TempDir(), "missing.exe")}

	t.Setenv("PATH", t.TempDir())
	if got := ContainerCLI(BackendPodman); got != BackendPodman {
		t.Errorf("ContainerCLI(podman) = %q, want bare %q", got, BackendPodman)
	}
}

func TestResolvePodmanFallback_Found(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only fallback loop")
	}
	fakePath := testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{})
	original := podmanWindowsPaths
	t.Cleanup(func() { podmanWindowsPaths = original })
	podmanWindowsPaths = []string{fakePath}

	got := ResolvePodmanFallback()
	if got != fakePath {
		t.Errorf("ResolvePodmanFallback() = %q, want %q", got, fakePath)
	}
}

func TestResolvePodmanFallback_NotFound(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only fallback loop")
	}
	original := podmanWindowsPaths
	t.Cleanup(func() { podmanWindowsPaths = original })
	podmanWindowsPaths = []string{filepath.Join(t.TempDir(), "missing.exe")}

	if got := ResolvePodmanFallback(); got != "" {
		t.Errorf("ResolvePodmanFallback() = %q, want empty", got)
	}
}
