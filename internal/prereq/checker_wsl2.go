package prereq

import (
	"os/exec"
	"runtime"

	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/wsl"
)

// checkWSL2 verifies that WSL2 is available on this system.
func (c *Checker) checkWSL2() CheckResult {
	name := "WSL2"

	if runtime.GOOS != "windows" {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "WSL2 is only available on Windows",
		}
	}

	if _, err := exec.LookPath("wsl.exe"); err != nil {
		isRequired := c.Backend == dockerbuild.BackendWSL2
		return CheckResult{
			Name:    name,
			Passed:  !isRequired,
			Warning: !isRequired,
			Message: "wsl.exe not found; install with: wsl --install",
		}
	}

	info, err := wsl.Detect()
	if err != nil || !info.Available {
		isRequired := c.Backend == dockerbuild.BackendWSL2
		return CheckResult{
			Name:    name,
			Passed:  !isRequired,
			Warning: !isRequired,
			Message: "WSL2 detected but no distros found; install with: wsl --install",
		}
	}

	distro, err := wsl.PickDistro(info, "")
	if err != nil {
		return CheckResult{
			Name:    name,
			Passed:  false,
			Message: err.Error(),
		}
	}

	return CheckResult{
		Name:    name,
		Passed:  true,
		Message: "WSL2 available (distro: " + distro + ")",
	}
}
