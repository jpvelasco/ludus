package prereq

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jpvelasco/ludus/internal/dockerbuild"
)

func (c *Checker) checkDocker() CheckResult {
	_, err := exec.LookPath("docker")
	if err != nil {
		if runtime.GOOS == "windows" {
			return CheckResult{
				Name:    "Docker",
				Passed:  true,
				Warning: true,
				Message: "docker not found in PATH (not needed for Windows client workflow)",
			}
		}
		if c.Backend != "" && c.Backend != dockerbuild.BackendDocker {
			return CheckResult{
				Name:    "Docker",
				Passed:  true,
				Warning: true,
				Message: "docker not found in PATH (not needed for " + c.Backend + " backend)",
			}
		}
		return CheckResult{
			Name:    "Docker",
			Passed:  false,
			Message: "docker not found in PATH",
		}
	}
	// Docker is in PATH — verify the daemon is running.
	// Use a timeout so we don't block forever if the CLI is installed but the daemon is not running.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		// If the user explicitly chose a different backend, Docker being down is just a warning.
		if c.Backend != "" && c.Backend != dockerbuild.BackendDocker {
			return CheckResult{
				Name:    "Docker",
				Passed:  true,
				Warning: true,
				Message: "docker daemon not running (not needed for " + c.Backend + " backend)",
			}
		}
		return CheckResult{
			Name:    "Docker",
			Passed:  false,
			Message: "docker found but daemon is not running; start Docker Desktop or the docker service",
		}
	}
	return CheckResult{
		Name:    "Docker",
		Passed:  true,
		Message: "docker daemon running",
	}
}

func (c *Checker) checkPodman() CheckResult {
	podmanBin := dockerbuild.BackendPodman
	if _, err := exec.LookPath(podmanBin); err != nil {
		// On Windows, check the default install location — winget puts podman
		// in Program Files but the current terminal may not have reloaded PATH.
		fallback := dockerbuild.ResolvePodmanFallback()
		if fallback == "" {
			if c.Backend == dockerbuild.BackendPodman {
				return CheckResult{
					Name:    "Podman",
					Passed:  false,
					Message: "podman not found in PATH; install with: winget install RedHat.Podman",
				}
			}
			return CheckResult{
				Name:    "Podman",
				Passed:  true,
				Warning: true,
				Message: "podman not found in PATH",
			}
		}
		podmanBin = fallback
	}
	// On Linux, podman runs natively without a VM machine.
	if runtime.GOOS == "linux" {
		return CheckResult{
			Name:    "Podman",
			Passed:  true,
			Message: "podman available (native)",
		}
	}
	// On Windows/macOS, podman needs a machine (VM).
	return c.checkPodmanMachine(podmanBin)
}

// checkPodmanMachine verifies that the podman VM is running and has sufficient
// resources for a UE5 engine build (Windows/macOS).
func (c *Checker) checkPodmanMachine(podmanBin string) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, podmanBin, "machine", "info").CombinedOutput()
	if err != nil || !strings.Contains(strings.ToLower(string(out)), "machinestate: running") {
		isRequired := c.Backend == dockerbuild.BackendPodman
		return CheckResult{
			Name:    "Podman",
			Passed:  !isRequired,
			Warning: !isRequired,
			Message: "podman found but machine not running; start with: podman machine start",
		}
	}

	// Machine is running — check that it has enough resources for a UE5 engine build.
	if warn := podmanMachineResourceWarning(podmanBin); warn != "" {
		return CheckResult{Name: "Podman", Passed: true, Warning: true, Message: warn}
	}
	return CheckResult{Name: "Podman", Passed: true, Message: "podman machine running"}
}

// podmanMachineResources holds the resource allocation of a podman machine.
type podmanMachineResources struct {
	DiskSize int `json:"DiskSize"` // GB
	Memory   int `json:"Memory"`   // MB
}

// podmanMachineResourceWarning runs `podman machine inspect` and returns a
// warning string if the machine is under-provisioned for UE5 engine builds.
// Returns "" when resources are sufficient or inspect is unavailable.
func podmanMachineResourceWarning(podmanBin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, podmanBin, "machine", "inspect").Output()
	if err != nil {
		return "" // inspect unavailable — don't block on best-effort check
	}
	res, err := parsePodmanMachineResources(out)
	if err != nil {
		return "" // can't parse — skip
	}
	return podmanResourceWarningFromResources(res)
}

const (
	podmanMinDiskGB = 300
	podmanMinMemMB  = 8 * 1024 // 8 GB in MB
)

// podmanResourceWarningFromResources returns a warning string if the given
// resource allocation is insufficient for a UE5 engine build, or "" if OK.
func podmanResourceWarningFromResources(res podmanMachineResources) string {
	var issues []string
	if res.DiskSize < podmanMinDiskGB {
		issues = append(issues, fmt.Sprintf("disk %d GB (need %d GB)", res.DiskSize, podmanMinDiskGB))
	}
	if res.Memory < podmanMinMemMB {
		issues = append(issues, fmt.Sprintf("memory %d MB (need %d MB)", res.Memory, podmanMinMemMB))
	}
	if len(issues) == 0 {
		return ""
	}
	return fmt.Sprintf("podman machine under-provisioned for UE5 engine builds: %s — "+
		"recreate with: podman machine stop && podman machine rm && "+
		"podman machine init --disk-size 400 --memory 12288 --cpus 8 && podman machine start",
		strings.Join(issues, ", "))
}

// parsePodmanMachineResources extracts resource fields from `podman machine inspect` JSON output.
// The output is a JSON array; resources are read from the first element.
func parsePodmanMachineResources(data []byte) (podmanMachineResources, error) {
	// `podman machine inspect` returns a JSON array of machine objects.
	// We only need the first machine's Resources field.
	var machines []struct {
		Resources podmanMachineResources `json:"Resources"`
	}
	if err := json.Unmarshal(data, &machines); err != nil || len(machines) == 0 {
		return podmanMachineResources{}, fmt.Errorf("cannot parse podman machine inspect output")
	}
	return machines[0].Resources, nil
}

// checkCrossArchEmulation verifies that the container runtime can build for the target
// architecture when it differs from the host. Cross-architecture builds
// (e.g. arm64 on an amd64 host) require QEMU user-mode emulation via binfmt_misc.
func (c *Checker) checkCrossArchEmulation() CheckResult {
	name := "Cross-Arch Emulation"

	if c.GameConfig == nil {
		return CheckResult{Name: name, Passed: true, Message: "no game config; skipping"}
	}

	targetArch := c.GameConfig.ResolvedArch()

	// On Apple Silicon + container backend, engine+game container builds are always
	// emulated (x86_64 QEMU) because Epic ships only an x86_64 Linux toolchain.
	// arm64/Graviton server output is still produced via cross-compilation inside it.
	if c.isAppleSiliconContainerBackend() {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "Apple Silicon + container backend: engine + game container builds use QEMU x86_64 emulation (due to Epic's toolchain). game.arch=arm64 still produces correct Graviton (arm64) server output via cross-compilation. Emulation has a performance cost.",
		}
	}

	if targetArch == runtime.GOARCH {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("native build (%s); no emulation needed", targetArch),
		}
	}

	cli, ok := c.resolveEmulationCLI()
	if !ok {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "no container runtime found; skipping cross-arch check",
		}
	}

	platform := "linux/" + targetArch
	if cli == dockerbuild.BackendPodman {
		return checkPodmanEmulation(name, targetArch, platform)
	}
	return checkBuildxEmulation(name, cli, targetArch, platform)
}

// resolveEmulationCLI returns the container CLI to use for cross-arch checks.
// If no CLI is configured, it probes for docker then podman. Returns ("", false)
// if no runtime is found.
func (c *Checker) resolveEmulationCLI() (string, bool) {
	if c.Backend != "" && c.Backend != dockerbuild.BackendNative {
		return c.Backend, true
	}
	if _, err := exec.LookPath(dockerbuild.BackendDocker); err == nil {
		return dockerbuild.BackendDocker, true
	}
	if _, err := exec.LookPath(dockerbuild.BackendPodman); err == nil {
		return dockerbuild.BackendPodman, true
	}
	return "", false
}

// isAppleSiliconContainerBackend reports whether we are on darwin/arm64 using
// a container backend. In this case engine (and game) container builds run
// under QEMU x86_64 emulation due to Epic's Linux toolchain requirements.
func (c *Checker) isAppleSiliconContainerBackend() bool {
	return runtime.GOOS == "darwin" &&
		runtime.GOARCH == "arm64" &&
		(c.Backend == dockerbuild.BackendDocker || c.Backend == dockerbuild.BackendPodman)
}

// checkPodmanEmulation checks QEMU emulation availability via podman.
func checkPodmanEmulation(name, targetArch, platform string) CheckResult {
	podmanBin := dockerbuild.BackendPodman
	if p := dockerbuild.ResolvePodmanFallback(); p != "" {
		podmanBin = p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, podmanBin, "info").Run(); err != nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "podman info unavailable; cannot verify cross-arch support",
		}
	}
	return CheckResult{
		Name:    name,
		Passed:  true,
		Warning: true,
		Message: fmt.Sprintf("podman detected; ensure QEMU emulation is registered for %s:\n"+
			"    podman run --rm --privileged tonistiigi/binfmt --install %s",
			platform, targetArch),
	}
}

// checkBuildxEmulation checks QEMU emulation availability via docker buildx.
func checkBuildxEmulation(name, cli, targetArch, platform string) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "buildx", "inspect", "--bootstrap").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("%s buildx not available; cannot verify cross-arch support", cli),
		}
	}
	if strings.Contains(string(out), platform) {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("%s can build for %s (QEMU emulation registered)", cli, platform),
		}
	}
	return CheckResult{
		Name:   name,
		Passed: false,
		Message: fmt.Sprintf("%s cannot build for %s on this %s host; "+
			"install QEMU emulation with:\n"+
			"    %s run --rm --privileged tonistiigi/binfmt --install %s",
			cli, platform, runtime.GOARCH, cli, targetArch),
	}
}

// checkMacOSContainerBuild verifies that macOS container build prerequisites are met.
// Skips silently on non-macOS platforms or non-container backends.
// Issues a warning (not failure) when the Linux toolchain is absent, since
// `ludus engine build` will fetch it automatically as a pre-flight step.
func (c *Checker) checkMacOSContainerBuild() CheckResult {
	name := "macOS Container Build"

	if c.EngineSourcePath == "" || (c.Backend != dockerbuild.BackendDocker && c.Backend != dockerbuild.BackendPodman) {
		return CheckResult{Name: name, Passed: true, Message: "skipped (not a macOS container build)"}
	}

	if !dockerbuild.LinuxToolchainPresent(c.EngineSourcePath, c.EngineVersion) {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "Linux toolchain not yet fetched — will be downloaded automatically on first engine build " +
				"(run 'ludus engine setup' to pre-fetch)",
		}
	}

	return CheckResult{Name: name, Passed: true, Message: "Linux toolchain present"}
}
