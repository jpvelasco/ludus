package prereq

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jpvelasco/ludus/internal/dockerbuild"
)

// minGoMajor / minGoMinor is the minimum host Go toolchain version required to
// build the Amazon GameLift server wrapper. The wrapper's Makefile invokes
// `go build -C src` (and `go test -C src`); the `-C` flag was introduced in
// Go 1.20, so an older toolchain fails mid-`container build` with
// "flag provided but not defined: -C".
const (
	minGoMajor = 1
	minGoMinor = 20
)

// errGoNotFound / errGoUnreadable distinguish "no go on PATH" (a hard failure
// for container builds) from "go present but its version couldn't be read or
// parsed" (a warning — we can't verify, but won't block).
var (
	errGoNotFound   = errors.New("go not found in PATH")
	errGoUnreadable = errors.New("go version could not be determined")
)

// checkGoVersion verifies the host Go toolchain is new enough to build the
// GameLift server wrapper. It is only relevant for the container backends
// (docker/podman), where `ludus container build` shells out to `go build`.
// For other backends it is a no-op pass, mirroring the other container-scoped
// checks (e.g. checkMacOSContainerBuild).
func (c *Checker) checkGoVersion() CheckResult {
	const name = "Go compiler version"

	// Treat default/empty backend as Docker. This ensures the check runs for the
	// common case of `ludus container build` (no --backend, config backend often
	// "native"/"wsl2") because ResolveContainerBackend returns "" but the actual
	// build (ContainerCLI etc.) defaults to Docker. See review feedback on #279.
	be := c.Backend
	if be == "" {
		be = dockerbuild.BackendDocker
	}
	if !dockerbuild.IsContainerBackend(be) {
		return CheckResult{Name: name, Passed: true, Message: "skipped — only required for container builds"}
	}

	major, minor, err := detectHostGoVersion()
	switch {
	case errors.Is(err, errGoNotFound):
		return CheckResult{Name: name, Passed: false,
			Message: fmt.Sprintf("go not found in PATH; container builds need Go >= %d.%d to build the GameLift wrapper", minGoMajor, minGoMinor)}
	case err != nil:
		return CheckResult{Name: name, Passed: true, Warning: true,
			Message: fmt.Sprintf("could not determine Go version; container builds need Go >= %d.%d for the GameLift wrapper", minGoMajor, minGoMinor)}
	case goVersionTooOld(major, minor):
		return CheckResult{Name: name, Passed: false,
			Message: fmt.Sprintf("go%d.%d found; GameLift wrapper build requires Go >= %d.%d (uses 'go build -C')", major, minor, minGoMajor, minGoMinor)}
	default:
		return CheckResult{Name: name, Passed: true,
			Message: fmt.Sprintf("go%d.%d found (>= %d.%d required for GameLift wrapper build)", major, minor, minGoMajor, minGoMinor)}
	}
}

// detectHostGoVersion runs `go version` and returns the parsed major/minor.
// It returns errGoNotFound if go is not on PATH, or errGoUnreadable if go ran
// but its version output could not be obtained or parsed.
func detectHostGoVersion() (major, minor int, err error) {
	if _, lookErr := exec.LookPath("go"); lookErr != nil {
		return 0, 0, errGoNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, runErr := exec.CommandContext(ctx, "go", "version").Output()
	if runErr != nil {
		return 0, 0, errGoUnreadable
	}

	major, minor, ok := parseGoMinorVersion(string(out))
	if !ok {
		return 0, 0, errGoUnreadable
	}
	return major, minor, nil
}

// goVersionTooOld reports whether (major, minor) is below the required minimum.
func goVersionTooOld(major, minor int) bool {
	if major != minGoMajor {
		return major < minGoMajor
	}
	return minor < minGoMinor
}

// parseGoMinorVersion extracts the major and minor version from `go version`
// output, e.g. "go version go1.25.10 linux/amd64" -> (1, 25, true). It returns
// ok=false if no recognizable goX.Y token is present.
func parseGoMinorVersion(versionOutput string) (major, minor int, ok bool) {
	for _, field := range strings.Fields(versionOutput) {
		num, found := strings.CutPrefix(field, "go")
		if !found || num == "" {
			continue
		}
		parts := strings.SplitN(num, ".", 3)
		if len(parts) < 2 {
			continue
		}
		maj, errMaj := strconv.Atoi(parts[0])
		min, errMin := strconv.Atoi(parts[1])
		if errMaj != nil || errMin != nil {
			continue
		}
		return maj, min, true
	}
	return 0, 0, false
}
