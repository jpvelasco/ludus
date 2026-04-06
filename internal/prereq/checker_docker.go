package prereq

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
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
	if err := exec.Command("docker", "info").Run(); err != nil {
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
	_, err := exec.LookPath("podman")
	if err != nil {
		return CheckResult{
			Name:    "Podman",
			Passed:  true,
			Warning: true,
			Message: "podman not found in PATH",
		}
	}
	// On Linux, podman runs natively without a VM machine.
	if runtime.GOOS == "linux" {
		return CheckResult{
			Name:    "Podman",
			Passed:  true,
			Message: "podman available (native)",
		}
	}
	// On Windows/macOS, podman needs a machine (WSL2 VM).
	out, err := exec.Command("podman", "machine", "info").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    "Podman",
			Passed:  true,
			Warning: true,
			Message: "podman found but machine may not be running; start with: podman machine start",
		}
	}
	if strings.Contains(string(out), "MachineState: Running") ||
		strings.Contains(string(out), "Currently running machine") {
		return CheckResult{
			Name:    "Podman",
			Passed:  true,
			Message: "podman machine running",
		}
	}
	return CheckResult{
		Name:    "Podman",
		Passed:  true,
		Warning: true,
		Message: "podman found but machine status unknown; ensure a machine is running",
	}
}

// checkCrossArchEmulation verifies that Docker can build for the target
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

	// A container runtime must be available for this check to be meaningful.
	cli := "docker"
	if _, err := exec.LookPath("docker"); err != nil {
		if _, err := exec.LookPath("podman"); err != nil {
			return CheckResult{
				Name:    name,
				Passed:  true,
				Warning: true,
				Message: "no container runtime found; skipping cross-arch check",
			}
		}
		cli = "podman"
	}

	// Map Go arch names to container platform strings.
	platform := "linux/" + targetArch

	// Podman doesn't have buildx; use `podman info` to check supported platforms.
	if cli == "podman" {
		out, err := exec.Command("podman", "info", "--format", "{{.Host.OCIRuntime.Name}}").CombinedOutput()
		if err != nil {
			return CheckResult{
				Name:    name,
				Passed:  true,
				Warning: true,
				Message: "podman info unavailable; cannot verify cross-arch support",
			}
		}
		_ = out // podman cross-arch via QEMU is available if binfmt is registered
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: fmt.Sprintf("podman detected; ensure QEMU emulation is registered for %s:\n"+
				"    podman run --rm --privileged tonistiigi/binfmt --install %s",
				platform, targetArch),
		}
	}

	out, err := exec.Command(cli, "buildx", "inspect", "--bootstrap").CombinedOutput()
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
