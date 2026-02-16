package prereq

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// CheckResult represents the result of a single prerequisite check.
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// Checker validates that all prerequisites for the Ludus pipeline are met.
type Checker struct {
	EngineSourcePath string
}

// NewChecker creates a new prerequisite checker.
func NewChecker(engineSourcePath string) *Checker {
	return &Checker{
		EngineSourcePath: engineSourcePath,
	}
}

// RunAll executes all prerequisite checks and returns the results.
func (c *Checker) RunAll() []CheckResult {
	var results []CheckResult

	results = append(results, c.checkOS())
	results = append(results, c.checkEngineSource())
	results = append(results, c.checkLyraContent())
	results = append(results, c.checkCommand("docker", "Docker"))
	results = append(results, c.checkCommand("aws", "AWS CLI"))
	results = append(results, c.checkCommand("git", "Git"))
	results = append(results, c.checkCommand("go", "Go compiler"))
	results = append(results, c.checkDiskSpace())
	results = append(results, c.checkMemory())

	return results
}

func (c *Checker) checkOS() CheckResult {
	if runtime.GOOS != "linux" {
		return CheckResult{
			Name:    "Operating System",
			Passed:  false,
			Message: fmt.Sprintf("Linux required, got %s", runtime.GOOS),
		}
	}
	return CheckResult{
		Name:    "Operating System",
		Passed:  true,
		Message: "Linux detected",
	}
}

func (c *Checker) checkCommand(cmd, name string) CheckResult {
	_, err := exec.LookPath(cmd)
	if err != nil {
		return CheckResult{
			Name:    name,
			Passed:  false,
			Message: fmt.Sprintf("%s not found in PATH", cmd),
		}
	}
	return CheckResult{
		Name:    name,
		Passed:  true,
		Message: fmt.Sprintf("%s found", cmd),
	}
}

func (c *Checker) checkEngineSource() CheckResult {
	if c.EngineSourcePath == "" {
		return CheckResult{
			Name:    "Engine Source",
			Passed:  false,
			Message: "engine sourcePath not configured",
		}
	}

	setupPath := filepath.Join(c.EngineSourcePath, "Setup.sh")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Engine Source",
			Passed:  false,
			Message: fmt.Sprintf("Setup.sh not found at %s", setupPath),
		}
	}

	return CheckResult{
		Name:    "Engine Source",
		Passed:  true,
		Message: fmt.Sprintf("found at %s", c.EngineSourcePath),
	}
}

func (c *Checker) checkLyraContent() CheckResult {
	if c.EngineSourcePath == "" {
		return CheckResult{
			Name:    "Lyra Content",
			Passed:  false,
			Message: "engine sourcePath not configured",
		}
	}

	// Check for the critical DefaultGameData asset that Lyra requires at startup
	contentDir := filepath.Join(c.EngineSourcePath, "Samples", "Games", "Lyra", "Content")
	gameData := filepath.Join(contentDir, "DefaultGameData.uasset")

	if _, err := os.Stat(gameData); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Lyra Content",
			Passed:  false,
			Message: fmt.Sprintf("Lyra Content not found at %s. "+
				"Epic does not distribute Lyra assets via GitHub. "+
				"Download 'Lyra Starter Game' from the Epic Games Launcher Marketplace, "+
				"then copy its Content/ folder to %s", contentDir, contentDir),
		}
	}

	return CheckResult{
		Name:    "Lyra Content",
		Passed:  true,
		Message: fmt.Sprintf("found at %s", contentDir),
	}
}

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
	const requiredGB = 100

	if freeGB < requiredGB {
		return CheckResult{
			Name:    "Disk Space",
			Passed:  false,
			Message: fmt.Sprintf("%d GB free, need %d GB", freeGB, requiredGB),
		}
	}

	return CheckResult{
		Name:    "Disk Space",
		Passed:  true,
		Message: fmt.Sprintf("%d GB free", freeGB),
	}
}

func (c *Checker) checkMemory() CheckResult {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return CheckResult{
			Name:    "Memory",
			Passed:  false,
			Message: fmt.Sprintf("cannot read /proc/meminfo: %v", err),
		}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				break
			}
			kB, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				break
			}
			totalGB := kB / (1024 * 1024)
			const requiredGB = 16
			if totalGB < requiredGB {
				return CheckResult{
					Name:    "Memory",
					Passed:  false,
					Message: fmt.Sprintf("%d GB total, need %d GB", totalGB, requiredGB),
				}
			}
			return CheckResult{
				Name:    "Memory",
				Passed:  true,
				Message: fmt.Sprintf("%d GB total", totalGB),
			}
		}
	}

	return CheckResult{
		Name:    "Memory",
		Passed:  false,
		Message: "could not parse /proc/meminfo",
	}
}
