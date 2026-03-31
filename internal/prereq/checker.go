package prereq

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/toolchain"
)

// CheckResult represents the result of a single prerequisite check.
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Warning bool   `json:"warning,omitempty"`
	Message string `json:"message"`
}

// Checker validates that all prerequisites for the Ludus pipeline are met.
type Checker struct {
	EngineSourcePath string
	EngineVersion    string
	Fix              bool
	GameConfig       *config.GameConfig
}

// NewChecker creates a new prerequisite checker.
func NewChecker(engineSourcePath, engineVersion string, fix bool, gameCfg *config.GameConfig) *Checker {
	return &Checker{
		EngineSourcePath: engineSourcePath,
		EngineVersion:    engineVersion,
		Fix:              fix,
		GameConfig:       gameCfg,
	}
}

// RunAll executes all prerequisite checks and returns the results.
func (c *Checker) RunAll() []CheckResult {
	var results []CheckResult

	results = append(results, c.checkOS())
	results = append(results, c.checkEngineSource())
	results = append(results, c.checkToolchain())
	results = append(results, c.checkGameContent())
	results = append(results, c.checkServerMap())
	results = append(results, c.checkDocker())
	results = append(results, c.checkCrossArchEmulation())
	results = append(results, c.checkCommand("aws", "AWS CLI"))
	results = append(results, c.checkAWSCredentials())
	results = append(results, c.checkCommand("git", "Git"))
	results = append(results, c.checkCommand("go", "Go compiler"))
	results = append(results, c.platformChecks()...)
	results = append(results, c.checkDiskSpace())
	results = append(results, c.checkMemory())

	return results
}

// Validate runs a set of checks and returns an error if any fail.
// Warnings are printed but do not cause failure.
func Validate(results []CheckResult) error {
	failed := 0
	for _, res := range results {
		if !res.Passed {
			fmt.Printf("  [FAIL] %s: %s\n", res.Name, res.Message)
			failed++
		} else if res.Warning {
			fmt.Printf("  [WARN] %s: %s\n", res.Name, res.Message)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d prerequisite check(s) failed; run 'ludus init' for full diagnostics", failed)
	}
	return nil
}

// CheckEngineReady validates prerequisites for engine build commands.
func (c *Checker) CheckEngineReady() []CheckResult {
	return []CheckResult{c.checkEngineSource()}
}

// CheckGameReady validates prerequisites for game build commands.
func (c *Checker) CheckGameReady() []CheckResult {
	return []CheckResult{c.checkEngineSource(), c.checkGameContent()}
}

// CheckDockerReady validates prerequisites for container build commands.
func (c *Checker) CheckDockerReady() []CheckResult {
	return []CheckResult{c.checkDocker(), c.checkCrossArchEmulation()}
}

// CheckPushReady validates prerequisites for push commands (container/engine push).
func (c *Checker) CheckPushReady() []CheckResult {
	return []CheckResult{c.checkDocker(), c.checkAWSCredentials()}
}

// CheckAWSReady validates prerequisites for deploy and connect commands.
func (c *Checker) CheckAWSReady() []CheckResult {
	return []CheckResult{c.checkAWSCredentials()}
}

func (c *Checker) checkOS() CheckResult {
	switch runtime.GOOS {
	case "linux":
		return CheckResult{
			Name:    "Operating System",
			Passed:  true,
			Message: "Linux detected",
		}
	case "windows":
		return CheckResult{
			Name:    "Operating System",
			Passed:  true,
			Message: "Windows detected (client builds only; server pipeline requires Linux)",
		}
	default:
		return CheckResult{
			Name:    "Operating System",
			Passed:  false,
			Message: fmt.Sprintf("unsupported OS: %s (need linux or windows)", runtime.GOOS),
		}
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

	setupFile := "Setup.sh"
	if runtime.GOOS == "windows" {
		setupFile = "Setup.bat"
	}

	setupPath := filepath.Join(c.EngineSourcePath, setupFile)
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Engine Source",
			Passed:  false,
			Message: fmt.Sprintf("%s not found at %s", setupFile, setupPath),
		}
	}

	return CheckResult{
		Name:    "Engine Source",
		Passed:  true,
		Message: fmt.Sprintf("found at %s", c.EngineSourcePath),
	}
}

func (c *Checker) checkToolchain() CheckResult {
	if c.EngineSourcePath == "" {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Warning: true,
			Message: "skipped (no engine source path)",
		}
	}

	tc := toolchain.CheckToolchain(c.EngineSourcePath, c.EngineVersion)

	// Version could not be detected at all
	if tc.EngineVersion == "" && tc.Required == nil {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Warning: true,
			Message: tc.Message,
		}
	}

	// Version detected but no known mapping
	if tc.Required == nil {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Warning: true,
			Message: tc.Message,
		}
	}

	// Toolchain found
	if tc.Found {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Message: tc.Message,
		}
	}

	// Not found on Windows — warning, or auto-fix if --fix
	if runtime.GOOS == "windows" {
		if c.Fix {
			return c.fixCrossCompileToolchain(tc)
		}
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Warning: true,
			Message: tc.Message + "; run with --fix to download and install",
		}
	}

	// Not found on Linux — fail
	msg := tc.Message
	if !c.Fix {
		msg += "; run with --fix for instructions"
	}
	return CheckResult{
		Name:    "Toolchain",
		Passed:  false,
		Message: msg,
	}
}

func (c *Checker) checkAWSCredentials() CheckResult {
	if _, err := exec.LookPath("aws"); err != nil {
		return CheckResult{Name: "AWS Credentials", Passed: true, Warning: true,
			Message: "skipped — AWS CLI not installed"}
	}
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return CheckResult{Name: "AWS Credentials", Passed: true, Warning: true,
			Message: "AWS credentials not configured or expired; run 'aws configure' or 'aws sso login'"}
	}
	var identity struct {
		Account string `json:"Account"`
		Arn     string `json:"Arn"`
	}
	if json.Unmarshal(out, &identity) != nil {
		return CheckResult{Name: "AWS Credentials", Passed: true, Warning: true,
			Message: "AWS CLI returned unexpected output"}
	}
	return CheckResult{Name: "AWS Credentials", Passed: true,
		Message: fmt.Sprintf("authenticated (account: %s)", identity.Account)}
}

func (c *Checker) checkServerMap() CheckResult {
	serverMap := ""
	if c.GameConfig != nil {
		serverMap = c.GameConfig.ServerMap
	}
	if serverMap == "" {
		return CheckResult{Name: "Server Map", Passed: true, Warning: true,
			Message: "skipped — no serverMap configured"}
	}

	contentDir := c.resolveContentDir()
	if contentDir == "" {
		return CheckResult{Name: "Server Map", Passed: true, Warning: true,
			Message: "skipped — could not determine project content directory"}
	}
	if _, err := os.Stat(contentDir); os.IsNotExist(err) {
		return CheckResult{Name: "Server Map", Passed: true, Warning: true,
			Message: "skipped — content directory does not exist yet"}
	}

	target := serverMap + ".umap"
	projectDir := filepath.Dir(contentDir) // e.g. .../Lyra

	// Search Content/ and Plugins/ — UE5 GameFeature plugins store maps
	// under Plugins/GameFeatures/<feature>/Content/Maps/.
	searchDirs := []string{contentDir, filepath.Join(projectDir, "Plugins")}

	for _, dir := range searchDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		var foundPath string
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.EqualFold(d.Name(), target) {
				foundPath = path
				return filepath.SkipAll
			}
			return nil
		})
		if foundPath != "" {
			rel, _ := filepath.Rel(projectDir, foundPath)
			return CheckResult{Name: "Server Map", Passed: true,
				Message: fmt.Sprintf("'%s' found at %s", serverMap, rel)}
		}
	}

	return CheckResult{Name: "Server Map", Passed: true, Warning: true,
		Message: fmt.Sprintf("'%s.umap' not found under %s; verify serverMap in ludus.yaml", serverMap, projectDir)}
}

func (c *Checker) resolveContentDir() string {
	if c.GameConfig != nil && c.GameConfig.ProjectPath != "" {
		return filepath.Join(filepath.Dir(c.GameConfig.ProjectPath), "Content")
	}
	pn := "Lyra"
	if c.GameConfig != nil && c.GameConfig.ProjectName != "" {
		pn = c.GameConfig.ProjectName
	}
	if pn == "Lyra" && c.EngineSourcePath != "" {
		return filepath.Join(c.EngineSourcePath, "Samples", "Games", "Lyra", "Content")
	}
	return ""
}
