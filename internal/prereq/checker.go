package prereq

import (
	"fmt"
	"os/exec"
	"runtime"
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

func (c *Checker) checkDiskSpace() CheckResult {
	// TODO: Implement actual disk space check using syscall.Statfs
	return CheckResult{
		Name:    "Disk Space",
		Passed:  false,
		Message: "not yet implemented",
	}
}

func (c *Checker) checkMemory() CheckResult {
	// TODO: Implement actual memory check using /proc/meminfo
	return CheckResult{
		Name:    "Memory (>= 16GB recommended)",
		Passed:  false,
		Message: "not yet implemented",
	}
}
