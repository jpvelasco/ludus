package prereq

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
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
		fallback := podmanWindowsFallback()
		if fallback == "" {
			if c.Backend == "podman" {
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
	// On Windows/macOS, podman needs a machine (WSL2 VM).
	out, err := exec.Command(podmanBin, "machine", "info").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    "Podman",
			Passed:  c.Backend != "podman",
			Warning: c.Backend != "podman",
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
		Passed:  c.Backend != "podman",
		Warning: c.Backend != "podman",
		Message: "podman found but machine not running; start with: podman machine start",
	}
}

// podmanWindowsFallback checks the default Podman install location on Windows.
// Returns the full path if found, empty string otherwise.
func podmanWindowsFallback() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	p := `C:\Program Files\RedHat\Podman\podman.exe`
	if _, err := exec.LookPath(p); err == nil {
		return p
	}
	return ""
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

	// Use the configured backend if set, otherwise probe for any available runtime.
	cli := c.Backend
	if cli == "" || cli == "native" {
		cli = "docker"
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
	}

	// Map Go arch names to container platform strings.
	platform := "linux/" + targetArch

	// Podman doesn't have buildx; use `podman info` to check supported platforms.
	if cli == "podman" {
		podmanBin := "podman"
		if p := podmanWindowsFallback(); p != "" {
			podmanBin = p
		}
		out, err := exec.Command(podmanBin, "info", "--format", "{{.Host.OCIRuntime.Name}}").CombinedOutput()
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

	buildxCtx, buildxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer buildxCancel()
	out, err := exec.CommandContext(buildxCtx, cli, "buildx", "inspect", "--bootstrap").CombinedOutput()
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
