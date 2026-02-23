package prereq

import (
	"fmt"
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
	results = append(results, c.checkDocker())
	results = append(results, c.checkCommand("aws", "AWS CLI"))
	results = append(results, c.checkCommand("git", "Git"))
	results = append(results, c.checkCommand("go", "Go compiler"))
	results = append(results, c.platformChecks()...)
	results = append(results, c.checkDiskSpace())
	results = append(results, c.checkMemory())

	return results
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

// checkGameContent validates game project content based on configuration.
// For Lyra with no explicit project path, delegates to the Lyra-specific check.
// For other projects, verifies the .uproject exists and optionally checks a content marker.
func (c *Checker) checkGameContent() CheckResult {
	projectName := "Lyra"
	if c.GameConfig != nil && c.GameConfig.ProjectName != "" {
		projectName = c.GameConfig.ProjectName
	}

	checkName := projectName + " Content"

	// If content validation is explicitly disabled, skip
	if c.GameConfig != nil && c.GameConfig.ContentValidation != nil && c.GameConfig.ContentValidation.Disabled {
		return CheckResult{
			Name:    checkName,
			Passed:  true,
			Warning: true,
			Message: "content validation disabled via config",
		}
	}

	// For Lyra with no explicit project path, use the Lyra-specific check
	if projectName == "Lyra" && (c.GameConfig == nil || c.GameConfig.ProjectPath == "") {
		return c.checkLyraContent()
	}

	// For custom projects, verify the .uproject exists
	if c.GameConfig == nil || c.GameConfig.ProjectPath == "" {
		return CheckResult{
			Name:    checkName,
			Passed:  false,
			Message: "game.projectPath not configured in ludus.yaml",
		}
	}

	projectPath := c.GameConfig.ProjectPath
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return CheckResult{
			Name:    checkName,
			Passed:  false,
			Message: fmt.Sprintf(".uproject not found at %s", projectPath),
		}
	}

	// Optionally check a content marker file
	if c.GameConfig.ContentValidation != nil && c.GameConfig.ContentValidation.ContentMarkerFile != "" {
		markerPath := filepath.Join(filepath.Dir(projectPath), c.GameConfig.ContentValidation.ContentMarkerFile)
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			return CheckResult{
				Name:    checkName,
				Passed:  false,
				Message: fmt.Sprintf("content marker file not found at %s", markerPath),
			}
		}
	}

	return CheckResult{
		Name:    checkName,
		Passed:  true,
		Message: fmt.Sprintf("project found at %s", projectPath),
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

	lyraDir := filepath.Join(c.EngineSourcePath, "Samples", "Games", "Lyra")
	contentDir := filepath.Join(lyraDir, "Content")
	gameData := filepath.Join(contentDir, "DefaultGameData.uasset")

	if _, err := os.Stat(gameData); os.IsNotExist(err) {
		return CheckResult{
			Name:   "Lyra Content",
			Passed: false,
			Message: fmt.Sprintf("Lyra Content not found at %s. "+
				"Epic does not distribute Lyra assets via GitHub. "+
				"Download 'Lyra Starter Game' from the Epic Games Launcher Marketplace, "+
				"then copy its Content/ folder to %s", contentDir, contentDir),
		}
	}

	// Verify plugin content dirs exist (common oversight: copying only top-level Content/)
	pluginContentDirs := []string{
		"ShooterCore",
		"ShooterMaps",
		"TopDownArena",
	}

	var missing []string
	for _, plugin := range pluginContentDirs {
		pluginDir := filepath.Join(lyraDir, "Plugins", "GameFeatures", plugin, "Content")
		if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
			missing = append(missing, plugin)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:   "Lyra Content",
			Passed: false,
			Message: fmt.Sprintf("top-level Content/ found but plugin content missing for: %s. "+
				"Copy the ENTIRE downloaded Lyra project over %s (overlay, not just Content/). "+
				"Each GameFeature plugin has its own Content/ directory required for cooking.",
				strings.Join(missing, ", "), lyraDir),
		}
	}

	return CheckResult{
		Name:    "Lyra Content",
		Passed:  true,
		Message: fmt.Sprintf("found at %s (including plugin content)", contentDir),
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

	// Not found on Windows — warning only (server builds are Linux-only)
	if runtime.GOOS == "windows" {
		return CheckResult{
			Name:    "Toolchain",
			Passed:  true,
			Warning: true,
			Message: tc.Message,
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
	return CheckResult{
		Name:    "Docker",
		Passed:  true,
		Message: "docker found",
	}
}
