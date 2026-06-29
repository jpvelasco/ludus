package prereq

import (
	"os/exec"

	"github.com/jpvelasco/ludus/internal/wrapper"
)

// checkMakeForWrapper verifies that `make` is installed when building the
// GameLift server wrapper for the given target requires it. The wrapper's
// native linux/amd64 build shells out to `make build`; every other target
// cross-compiles with `go build` and needs no make. This mirrors checkGoVersion
// (a hard failure when the required tool is absent) so a missing make fails
// fast at readiness time instead of deep inside `deploy anywhere`/`ec2` with a
// raw `exec: "make": executable file not found in $PATH`.
func (c *Checker) checkMakeForWrapper(targetOS, arch string) CheckResult {
	const name = "make"

	if !wrapper.UsesMake(targetOS, arch) {
		return CheckResult{Name: name, Passed: true, Message: "skipped — only required for native linux/amd64 wrapper build"}
	}

	if _, err := exec.LookPath("make"); err != nil {
		return CheckResult{Name: name, Passed: false,
			Message: "make not found in PATH; the GameLift wrapper build needs it — install build-essential (Debian/Ubuntu), make (Fedora/RHEL), or the equivalent for your distro"}
	}

	return CheckResult{Name: name, Passed: true, Message: "make found"}
}

// CheckWrapperBuildReady validates host prerequisites for deploy targets that
// build the GameLift server wrapper from source (anywhere, ec2). targetOS/arch
// are the wrapper build target. Currently this is the make check (needed only
// for the native linux/amd64 path); the Go toolchain is validated separately at
// init via RunAll.
func (c *Checker) CheckWrapperBuildReady(targetOS, arch string) []CheckResult {
	return []CheckResult{c.checkMakeForWrapper(targetOS, arch)}
}
