package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/toolchain"
)

// checkToolchainConsistency verifies the toolchain version in the environment
// matches what the engine expects.
func checkToolchainConsistency(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Toolchain Consistency"}

	if cfg.Engine.SourcePath == "" {
		d.status = "ok"
		d.message = "skipped — no engine source configured"
		return d
	}

	tc := toolchain.CheckToolchain(cfg.Engine.SourcePath, cfg.Engine.Version)
	if tc.Required == nil {
		d.status = "ok"
		d.message = "no known toolchain requirement for this engine version"
		return d
	}

	if !tc.Found {
		d.status = "warn"
		d.message = fmt.Sprintf("required %s not found; run 'ludus init --fix'", tc.Required.SDKVersion)
		return d
	}

	// Check if LINUX_MULTIARCH_ROOT points to the right version
	lmr := os.Getenv("LINUX_MULTIARCH_ROOT")
	if lmr != "" && !strings.Contains(lmr, tc.Required.SDKVersion) {
		d.status = "warn"
		d.message = fmt.Sprintf("LINUX_MULTIARCH_ROOT points to %s but engine requires %s; restart terminal after toolchain install",
			filepath.Base(lmr), tc.Required.SDKVersion)
		return d
	}

	d.status = "ok"
	d.message = fmt.Sprintf("%s found and matches engine requirement", tc.Required.SDKVersion)
	return d
}

// checkDiskSpace warns if available disk space is low for builds.
func checkDiskSpace(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Disk Space"}

	checkPath := cfg.Engine.SourcePath
	if checkPath == "" {
		var err error
		checkPath, err = os.Getwd()
		if err != nil {
			d.status = "ok"
			d.message = "could not determine path to check"
			return d
		}
	}

	freeGB := getFreeDiskGB(checkPath)
	if freeGB < 0 {
		d.status = "ok"
		d.message = "could not determine free space"
		return d
	}

	if freeGB < 50 {
		d.status = "fail"
		d.message = fmt.Sprintf("%.0f GB free — builds require 50-100 GB free space", freeGB)
		return d
	}
	if freeGB < 100 {
		d.status = "warn"
		d.message = fmt.Sprintf("%.0f GB free — consider freeing space (100 GB recommended for builds)", freeGB)
		return d
	}

	d.status = "ok"
	d.message = fmt.Sprintf("%.0f GB free", freeGB)
	return d
}

// checkGitState checks for uncommitted changes in the engine source (which
// can cause UBT to rebuild everything).
func checkGitState() diagnostic {
	d := diagnostic{name: "Git Status"}

	if _, err := exec.LookPath("git"); err != nil {
		d.status = "ok"
		d.message = "git not available"
		return d
	}

	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		d.status = "ok"
		d.message = "not in a git repository"
		return d
	}

	lines := strings.TrimSpace(string(out))
	if lines == "" {
		d.status = "ok"
		d.message = "working tree clean"
		return d
	}

	count := len(strings.Split(lines, "\n"))
	d.status = "ok"
	d.message = fmt.Sprintf("%d modified file(s) in working tree", count)
	return d
}

// dockerNotInstalledDiagnostic returns the appropriate diagnostic when docker is absent.
func dockerNotInstalledDiagnostic() diagnostic {
	d := diagnostic{name: "Docker Daemon"}
	if runtime.GOOS == "windows" {
		d.status = "ok"
		d.message = "skipped — not needed for Windows client workflow"
	} else {
		d.status = "warn"
		d.message = "docker not installed"
	}
	return d
}
