package prereq

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestCheckGoVersion_TooOld(t *testing.T) {
	testsupport.FakeTool(t, "go", testsupport.ToolBehavior{Stdout: "go version go1.19.5 windows/amd64"})
	res := (&Checker{Backend: "docker"}).checkGoVersion()
	if res.Passed || !strings.Contains(res.Message, "requires Go") {
		t.Fatalf("checkGoVersion() = %+v", res)
	}
}

func TestCheckGoVersion_Unreadable(t *testing.T) {
	testsupport.FakeTool(t, "go", testsupport.ToolBehavior{ExitCode: 1})
	res := (&Checker{Backend: "docker"}).checkGoVersion()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "could not determine Go version") {
		t.Fatalf("checkGoVersion() = %+v", res)
	}
}

func TestCheckWSL2_NoDistros(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("wsl.exe stubbing requires Windows")
	}
	testsupport.FakeTool(t, "wsl.exe", testsupport.ToolBehavior{ExitCode: 1})
	res := (&Checker{Backend: "wsl2"}).checkWSL2()
	if res.Passed || !strings.Contains(res.Message, "no distros found") {
		t.Fatalf("checkWSL2() = %+v", res)
	}
}

func TestCheckContainerRuntime_PodmanBackendNotReady(t *testing.T) {
	t.Setenv("PATH", "")
	res := (&Checker{Backend: "podman"}).checkContainerRuntime()
	if res.Passed || !strings.Contains(res.Message, "podman backend selected but not ready") {
		t.Fatalf("checkContainerRuntime() = %+v", res)
	}
}

func TestCheckContainerRuntime_DockerBackendNotReady(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	res := (&Checker{Backend: "docker"}).checkContainerRuntime()
	if res.Passed || !strings.Contains(res.Message, "docker backend selected but not ready") {
		t.Fatalf("checkContainerRuntime() = %+v", res)
	}
}

func TestCheckContainerRuntime_DockerBackendReady(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{})
	res := (&Checker{Backend: "docker"}).checkContainerRuntime()
	if !res.Passed || res.Warning || !strings.Contains(res.Message, "docker daemon running") {
		t.Fatalf("checkContainerRuntime() = %+v", res)
	}
}

func TestCheckDocker_DaemonDownOtherBackend(t *testing.T) {
	testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
	res := (&Checker{Backend: "podman"}).checkDocker()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "not needed for podman backend") {
		t.Fatalf("checkDocker() = %+v", res)
	}
}

func TestCheckPodman_MachineNotRunning(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("podman machine path only applies on Windows/macOS")
	}
	testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{ExitCode: 1})
	res := (&Checker{}).checkPodman()
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "machine not running") {
		t.Fatalf("checkPodman() = %+v", res)
	}
}

func TestCheckBuildxEmulation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stdout string
		pass   bool
	}{
		{"registered", "platforms: linux/arm64, linux/amd64", true},
		{"missing platform", "platforms: linux/amd64", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{Stdout: tt.stdout})
			res := checkBuildxEmulation("Cross-Arch Emulation", "docker", "arm64", "linux/arm64")
			if res.Passed != tt.pass {
				t.Fatalf("checkBuildxEmulation() = %+v", res)
			}
		})
	}
}

func TestCheckPodmanEmulation_InfoUnavailable(t *testing.T) {
	testsupport.FakeTool(t, "podman", testsupport.ToolBehavior{ExitCode: 1})
	res := checkPodmanEmulation("Cross-Arch Emulation", "arm64", "linux/arm64")
	if !res.Passed || !res.Warning || !strings.Contains(res.Message, "podman info unavailable") {
		t.Fatalf("checkPodmanEmulation() = %+v", res)
	}
}

func TestCheckLyraContent_AllPresent(t *testing.T) {
	root := testsupport.FakeEngineTree(t)
	content := filepath.Join(root, "Samples", "Games", "Lyra", "Content")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "DefaultGameData.uasset"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, plugin := range []string{"ShooterCore", "ShooterMaps", "TopDownArena"} {
		dir := filepath.Join(root, "Samples", "Games", "Lyra", "Plugins", "GameFeatures", plugin, "Content")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	res := (&Checker{EngineSourcePath: root}).checkLyraContent()
	if !res.Passed || !strings.Contains(res.Message, "found at") {
		t.Fatalf("checkLyraContent() = %+v", res)
	}
}

func TestCheckCandidates_NoneMatch(t *testing.T) {
	if got := checkCandidates([]string{t.TempDir(), filepath.Join(t.TempDir(), "missing")}); got != "" {
		t.Fatalf("checkCandidates() = %q, want empty", got)
	}
}

func TestDiscoverLyraContent_NoMatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OneDrive", "")
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if got := discoverLyraContent(); got != "" {
		t.Fatalf("discoverLyraContent() = %q, want empty", got)
	}
}

func TestResolveEmulationCLI(t *testing.T) {
	if cli, ok := (&Checker{Backend: "podman"}).resolveEmulationCLI(); !ok || cli != "podman" {
		t.Fatalf("resolveEmulationCLI(backend) = (%q, %v)", cli, ok)
	}

	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {},
		"podman": {},
	})
	if cli, ok := (&Checker{Backend: ""}).resolveEmulationCLI(); !ok || cli != "docker" {
		t.Fatalf("resolveEmulationCLI(probe) = (%q, %v)", cli, ok)
	}

	t.Setenv("PATH", t.TempDir())
	if cli, ok := (&Checker{Backend: ""}).resolveEmulationCLI(); ok || cli != "" {
		t.Fatalf("resolveEmulationCLI(none) = (%q, %v)", cli, ok)
	}
}
