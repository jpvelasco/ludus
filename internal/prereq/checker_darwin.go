//go:build darwin

package prereq

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/jpvelasco/ludus/internal/toolchain"
)

func (c *Checker) checkDiskSpace() CheckResult {
	checkPath := c.EngineSourcePath
	if checkPath == "" {
		checkPath = "."
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(checkPath, &stat); err != nil {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("failed to check disk space: %v", err),
		}
	}

	freeGB := (stat.Bavail * uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	return diskSpaceResult(freeGB, c.Backend)
}

func (c *Checker) platformChecks() []CheckResult {
	// Native builds on macOS require full Xcode, not just Command Line Tools.
	// Container builds don't need Xcode at all — skip the check for them.
	if c.Backend == dockerbuild.BackendDocker || c.Backend == dockerbuild.BackendPodman {
		return nil
	}
	return []CheckResult{c.checkXcode()}
}

// checkXcode verifies that full Xcode (not just Command Line Tools) is installed
// on macOS. Native UE5 engine builds invoke xcodebuild internally — CLT-only
// installations fail deep in UBT with "Platform Mac is not a valid platform"
// instead of a clear diagnostic.
func (c *Checker) checkXcode() CheckResult {
	name := "Xcode"

	if _, err := exec.LookPath("xcodebuild"); err != nil {
		return CheckResult{
			Name:    name,
			Passed:  false,
			Message: "xcodebuild not found — install full Xcode from the App Store (Command Line Tools alone are not sufficient for native UE5 builds)",
		}
	}

	// `xcodebuild -version` exits 0 for full Xcode, non-zero for CLT-only.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xcodebuild", "-version").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    name,
			Passed:  false,
			Message: "xcodebuild found but is not functional — install full Xcode from the App Store, then run: sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer",
		}
	}

	// CLT-only installations report "xcode-select: error: tool 'xcodebuild' requires Xcode"
	outStr := strings.ToLower(string(out))
	if strings.Contains(outStr, "requires xcode") || strings.Contains(outStr, "error:") {
		return CheckResult{
			Name:    name,
			Passed:  false,
			Message: "Command Line Tools detected but full Xcode is required for native UE5 builds — install Xcode from the App Store, then run: sudo xcode-select --switch /Applications/Xcode.app/Contents/Developer",
		}
	}

	return CheckResult{
		Name:    name,
		Passed:  true,
		Message: fmt.Sprintf("Xcode installed (%s)", strings.TrimSpace(strings.Split(string(out), "\n")[0])),
	}
}

// fixCrossCompileToolchain is a no-op on macOS.
func (c *Checker) fixCrossCompileToolchain(_ toolchain.CheckResult) CheckResult {
	return CheckResult{
		Name:    "Toolchain",
		Passed:  true,
		Warning: true,
		Message: "cross-compile toolchain install not supported on this platform",
	}
}
