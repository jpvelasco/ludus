package prereq

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestCheckDocker_BackendDowngradeToWarning(t *testing.T) {
	// When using podman backend, Docker daemon being down should be a warning, not failure.
	c := &Checker{Backend: "podman"}
	result := c.checkDocker()

	// We can't guarantee Docker is or isn't available in test environments,
	// but if Docker is in PATH but daemon is down, the backend field should downgrade to warning.
	if result.Name != "Docker" {
		t.Errorf("expected name 'Docker', got: %s", result.Name)
	}
	// When backend is podman, Docker checks must never be Passed=false.
	if !result.Passed {
		t.Errorf("expected Docker check to pass (as warning) with podman backend, got: %s", result.Message)
	}
}

func TestCheckDocker_NoBackend(t *testing.T) {
	// Without a backend set, Docker check may fail if daemon is down.
	c := &Checker{}
	result := c.checkDocker()
	if result.Name != "Docker" {
		t.Errorf("expected name 'Docker', got: %s", result.Name)
	}
	// On Windows, if docker isn't found, it still passes as warning.
	// If it's found but daemon is down, it fails. Either way, we just verify no panic.
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckPodman_BackendPodmanNotFound(t *testing.T) {
	// When backend is podman but podman is not in PATH, it should fail —
	// unless the Windows fallback finds Podman at its default install location.
	c := &Checker{Backend: "podman"}
	t.Setenv("PATH", "")
	result := c.checkPodman()
	if result.Passed || strings.Contains(result.Message, "podman found") {
		// Podman found via fallback path or other mechanism.
		t.Skip("podman found despite empty PATH (Windows fallback or system lookup)")
	}
	if !strings.Contains(result.Message, "not found in PATH") {
		t.Errorf("expected 'not found' message, got: %s", result.Message)
	}
}

func TestCheckPodman_BackendDockerNotFound(t *testing.T) {
	// When backend is docker and podman is not in PATH, podman check should be a warning.
	c := &Checker{Backend: "docker"}
	t.Setenv("PATH", "")
	result := c.checkPodman()
	if !result.Passed {
		t.Errorf("expected pass (warning) when backend is docker and podman not found, got: %s", result.Message)
	}
	if !result.Warning {
		t.Errorf("expected warning flag set")
	}
}

func TestCheckMacOSContainerBuild_NoEngineSource(t *testing.T) {
	c := &Checker{Backend: "podman"}
	result := c.checkMacOSContainerBuild()
	if result.Name != "macOS Container Build" {
		t.Errorf("expected name 'macOS Container Build', got: %s", result.Name)
	}
	if !result.Passed {
		t.Errorf("expected pass (skip) with no engine source, got: %s", result.Message)
	}
	if result.Warning {
		t.Errorf("expected no warning when skipped due to no engine source")
	}
}

func TestCheckMacOSContainerBuild_NonContainerBackend(t *testing.T) {
	c := &Checker{Backend: "native", EngineSourcePath: "/some/path"}
	result := c.checkMacOSContainerBuild()
	if !result.Passed {
		t.Errorf("expected pass (skip) for native backend, got: %s", result.Message)
	}
	if result.Warning {
		t.Errorf("native backend should not warn")
	}
}

func TestCheckMacOSContainerBuild_ToolchainMissing(t *testing.T) {
	root := t.TempDir()
	c := &Checker{
		Backend:          "podman",
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		GameConfig:       &config.GameConfig{Arch: "arm64"},
	}
	result := c.checkMacOSContainerBuild()
	if result.Name != "macOS Container Build" {
		t.Errorf("unexpected name: %s", result.Name)
	}
	if !result.Passed {
		t.Errorf("expected pass+warning (not failure) for missing toolchain, got: %s", result.Message)
	}
	if !result.Warning {
		t.Errorf("expected warning flag for missing toolchain")
	}
	if !strings.Contains(result.Message, "Linux toolchain") {
		t.Errorf("expected 'Linux toolchain' in message, got: %s", result.Message)
	}
}

func TestCheckMacOSContainerBuild_ToolchainPresent(t *testing.T) {
	root := t.TempDir()
	sdkDir := filepath.Join(root, "Engine", "Extras", "ThirdPartyNotUE", "SDKs", "HostLinux", "Linux_x64", "v26_clang-20.1.8-rockylinux8")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Checker{
		Backend:          "podman",
		EngineSourcePath: root,
		EngineVersion:    "5.7",
		GameConfig:       &config.GameConfig{Arch: "arm64"},
	}
	result := c.checkMacOSContainerBuild()
	if !result.Passed || result.Warning {
		t.Errorf("expected clean pass when toolchain present: passed=%v warning=%v message=%s",
			result.Passed, result.Warning, result.Message)
	}
}

func TestCheckMacOSContainerBuild_DockerBackend(t *testing.T) {
	root := t.TempDir()
	c := &Checker{
		Backend:          "docker",
		EngineSourcePath: root,
		EngineVersion:    "5.7",
	}
	result := c.checkMacOSContainerBuild()
	// Docker backend with missing toolchain → warning
	if !result.Passed {
		t.Errorf("expected pass+warning for docker backend, got failure: %s", result.Message)
	}
	if !result.Warning {
		t.Errorf("expected warning for docker backend with missing toolchain")
	}
}

func TestCheckDocker_NotFoundWindowsBranch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific branch")
	}
	// Empty PATH makes LookPath fail; on Windows the check degrades to a
	// warning because Docker is optional for the client workflow.
	t.Setenv("PATH", t.TempDir())
	c := &Checker{}
	result := c.checkDocker()
	if !result.Passed || !result.Warning {
		t.Errorf("checkDocker() = %+v, want pass+warning on Windows", result)
	}
	if !strings.Contains(result.Message, "not needed for Windows") {
		t.Errorf("message %q missing Windows guidance", result.Message)
	}
}

func TestResolveEmulationCLI_PodmanProbe(t *testing.T) {
	// Restrict PATH to the podman stub so the docker probe misses.
	dir := testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{"podman": {}})
	t.Setenv("PATH", dir)
	c := &Checker{}
	cli, ok := c.resolveEmulationCLI()
	if !ok || cli != dockerbuild.BackendPodman {
		t.Errorf("resolveEmulationCLI() = (%q, %v), want podman found", cli, ok)
	}
}

func TestCheckPodmanMachine_Running(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("podman machine check is Windows/macOS-only")
	}
	// "MachineState: running" output makes checkPodmanMachine take the
	// running branch; unparsable inspect JSON yields no resource warning.
	testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{Stdout: "MachineState: running"})
	c := &Checker{}
	result := c.checkPodman()
	if !result.Passed {
		t.Errorf("checkPodman() = %+v, want pass", result)
	}
	if !strings.Contains(result.Message, "running") {
		t.Errorf("message %q missing 'running'", result.Message)
	}
}

func TestPodmanMachineResourceWarning(t *testing.T) {
	t.Run("inspect fails returns empty", func(t *testing.T) {
		path := testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{ExitCode: 1})
		if got := podmanMachineResourceWarning(path); got != "" {
			t.Errorf("podmanMachineResourceWarning() = %q, want empty on failure", got)
		}
	})

	t.Run("under-provisioned warns", func(t *testing.T) {
		path := testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{
			Stdout: `[{"Resources":{"DiskSize":100,"Memory":4096}}]`,
		})
		got := podmanMachineResourceWarning(path)
		if !strings.Contains(got, "under-provisioned") {
			t.Errorf("podmanMachineResourceWarning() = %q, want under-provisioned warning", got)
		}
	})

	t.Run("sufficient resources empty", func(t *testing.T) {
		path := testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{
			Stdout: `[{"Resources":{"DiskSize":1100,"Memory":12288}}]`,
		})
		if got := podmanMachineResourceWarning(path); got != "" {
			t.Errorf("podmanMachineResourceWarning() = %q, want empty when sufficient", got)
		}
	})
}

func TestCheckCrossArchEmulation_NoRuntimeSkips(t *testing.T) {
	// Use the arch opposite to the host so the native-build early return
	// (arm64 host targeting arm64) never fires before the no-runtime branch.
	arch := "arm64"
	if runtime.GOARCH == "arm64" {
		arch = "amd64"
	}
	t.Setenv("PATH", t.TempDir())
	c := &Checker{GameConfig: &config.GameConfig{Arch: arch}}
	result := c.checkCrossArchEmulation()
	if !result.Passed || !result.Warning {
		t.Errorf("checkCrossArchEmulation() = %+v, want pass+warning when no runtime", result)
	}
	if !strings.Contains(result.Message, "skipping") {
		t.Errorf("message %q missing 'skipping'", result.Message)
	}
}

func TestCheckPodmanEmulation_Detected(t *testing.T) {
	// On Windows, ResolvePodmanFallback checks the default install location and
	// takes precedence over the PATH stub; skip when a real podman is installed.
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(`C:\Program Files\RedHat\Podman\podman.exe`); err == nil {
			t.Skip("real podman install shadows the PATH stub")
		}
	}
	testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{})
	result := checkPodmanEmulation("Cross-Arch Emulation", "amd64", "linux/amd64")
	if !result.Passed || !result.Warning {
		t.Errorf("checkPodmanEmulation() = %+v, want pass+warning", result)
	}
	if !strings.Contains(result.Message, "ensure QEMU") {
		t.Errorf("message %q missing QEMU guidance", result.Message)
	}
}

func TestCheckBuildxEmulation_Unavailable(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	result := checkBuildxEmulation("Cross-Arch Emulation", dockerbuild.BackendDocker, "arm64", "linux/arm64")
	if !result.Passed || !result.Warning {
		t.Errorf("checkBuildxEmulation() = %+v, want pass+warning when buildx unavailable", result)
	}
	if !strings.Contains(result.Message, "buildx not available") {
		t.Errorf("message %q missing buildx guidance", result.Message)
	}
}
