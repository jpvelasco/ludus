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

	// Docker must be available for this check to be meaningful.
	if _, err := exec.LookPath("docker"); err != nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "docker not found; skipping cross-arch check",
		}
	}

	// Map Go arch names to Docker platform strings.
	dockerPlatform := "linux/" + targetArch

	out, err := exec.Command("docker", "buildx", "inspect", "--bootstrap").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "docker buildx not available; cannot verify cross-arch support",
		}
	}

	if strings.Contains(string(out), dockerPlatform) {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("Docker can build for %s (QEMU emulation registered)", dockerPlatform),
		}
	}

	return CheckResult{
		Name:   name,
		Passed: false,
		Message: fmt.Sprintf("Docker cannot build for %s on this %s host; "+
			"install QEMU emulation with:\n"+
			"    docker run --rm --privileged tonistiigi/binfmt --install %s",
			dockerPlatform, runtime.GOARCH, targetArch),
	}
}
