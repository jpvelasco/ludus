package prereq

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/devrecon/ludus/internal/dockerbuild"
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
		if c.Backend != "" && c.Backend != "docker" {
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
	podmanBin := "podman"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, podmanBin, "machine", "info").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    "Podman",
			Passed:  c.Backend != dockerbuild.BackendPodman,
			Warning: c.Backend != dockerbuild.BackendPodman,
			Message: "podman found but machine may not be running; start with: podman machine start",
		}
	}
	// podman machine info outputs YAML: "machinestate: Running"
	lower := strings.ToLower(string(out))
	if strings.Contains(lower, "machinestate: running") {
		return CheckResult{
			Name:    "Podman",
			Passed:  true,
			Message: "podman machine running",
		}
	}
	return CheckResult{
		Name:    "Podman",
		Passed:  c.Backend != dockerbuild.BackendPodman,
		Warning: c.Backend != dockerbuild.BackendPodman,
		Message: "podman found but machine not running; start with: podman machine start",
	}
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
	if targetArch == runtime.GOARCH {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("native build (%s); no emulation needed", targetArch),
		}
	}

	cli, ok := resolveEmulationCLI(c.Backend)
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
func resolveEmulationCLI(backend string) (string, bool) {
	if backend != "" && backend != dockerbuild.BackendNative {
		return backend, true
	}
	if _, err := exec.LookPath(dockerbuild.BackendDocker); err == nil {
		return dockerbuild.BackendDocker, true
	}
	if _, err := exec.LookPath(dockerbuild.BackendPodman); err == nil {
		return dockerbuild.BackendPodman, true
	}
	return "", false
}

// checkPodmanEmulation checks QEMU emulation availability via podman.
func checkPodmanEmulation(name, targetArch, platform string) CheckResult {
	podmanBin := "podman"
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
