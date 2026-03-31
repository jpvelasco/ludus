package prereq

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// checkGameContent validates game project content based on configuration.
func (c *Checker) checkGameContent() CheckResult {
	projectName := "Lyra"
	if c.GameConfig != nil && c.GameConfig.ProjectName != "" {
		projectName = c.GameConfig.ProjectName
	}

	checkName := projectName + " Content"

	if c.GameConfig != nil && c.GameConfig.ContentValidation != nil && c.GameConfig.ContentValidation.Disabled {
		return CheckResult{Name: checkName, Passed: true, Warning: true,
			Message: "content validation disabled via config"}
	}

	if projectName == "Lyra" && (c.GameConfig == nil || c.GameConfig.ProjectPath == "") {
		return c.checkLyraContent()
	}

	return c.checkCustomProjectContent(checkName)
}

// checkCustomProjectContent validates a non-Lyra project's content.
func (c *Checker) checkCustomProjectContent(checkName string) CheckResult {
	if c.GameConfig == nil || c.GameConfig.ProjectPath == "" {
		return CheckResult{Name: checkName, Passed: false,
			Message: "game.projectPath not configured in ludus.yaml"}
	}

	projectPath := c.GameConfig.ProjectPath
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return CheckResult{Name: checkName, Passed: false,
			Message: fmt.Sprintf(".uproject not found at %s", projectPath)}
	}

	if c.GameConfig.ContentValidation != nil && c.GameConfig.ContentValidation.ContentMarkerFile != "" {
		markerPath := filepath.Join(filepath.Dir(projectPath), c.GameConfig.ContentValidation.ContentMarkerFile)
		if _, err := os.Stat(markerPath); os.IsNotExist(err) {
			return CheckResult{Name: checkName, Passed: false,
				Message: fmt.Sprintf("content marker file not found at %s", markerPath)}
		}
	}

	return CheckResult{Name: checkName, Passed: true,
		Message: fmt.Sprintf("project found at %s", projectPath)}
}

// lyraContentState captures the state of Lyra content directories.
type lyraContentState struct {
	lyraDir        string
	contentDir     string
	topMissing     bool
	missingPlugins []string
}

func (c *Checker) checkLyraContent() CheckResult {
	if c.EngineSourcePath == "" {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: "engine sourcePath not configured"}
	}

	state := c.detectLyraContentState()

	// Content is fully present
	if !state.topMissing && len(state.missingPlugins) == 0 {
		return CheckResult{Name: "Lyra Content", Passed: true,
			Message: fmt.Sprintf("found at %s (including plugin content)", state.contentDir)}
	}

	return c.resolveLyraContentFix(state)
}

func (c *Checker) detectLyraContentState() lyraContentState {
	lyraDir := filepath.Join(c.EngineSourcePath, "Samples", "Games", "Lyra")
	contentDir := filepath.Join(lyraDir, "Content")
	gameData := filepath.Join(contentDir, "DefaultGameData.uasset")

	s := lyraContentState{lyraDir: lyraDir, contentDir: contentDir}

	if _, err := os.Stat(gameData); os.IsNotExist(err) {
		s.topMissing = true
		return s
	}

	for _, plugin := range []string{"ShooterCore", "ShooterMaps", "TopDownArena"} {
		pluginDir := filepath.Join(lyraDir, "Plugins", "GameFeatures", plugin, "Content")
		if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
			s.missingPlugins = append(s.missingPlugins, plugin)
		}
	}
	return s
}

func (c *Checker) resolveLyraContentFix(s lyraContentState) CheckResult {
	contentSourcePath := ""
	if c.GameConfig != nil {
		contentSourcePath = c.GameConfig.ContentSourcePath
	}

	// Try auto-discovery if no source configured
	if contentSourcePath == "" {
		contentSourcePath = discoverLyraContent()
	}

	if s.topMissing {
		return c.fixMissingTopContent(s, contentSourcePath)
	}
	return c.fixMissingPluginContent(s, contentSourcePath)
}

func (c *Checker) fixMissingTopContent(s lyraContentState, contentSourcePath string) CheckResult {
	if contentSourcePath == "" {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: fmt.Sprintf("Lyra Content not found at %s. "+
				"Download 'Lyra Starter Game' from the Epic Games Launcher, "+
				"then either copy it to %s manually, or set game.contentSourcePath "+
				"in ludus.yaml and run ludus init --fix", s.contentDir, s.lyraDir)}
	}

	if !c.Fix {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: fmt.Sprintf("Lyra Content not found at %s; "+
				"run with --fix to overlay from %s", s.contentDir, contentSourcePath)}
	}
	return c.overlayLyraContent(contentSourcePath, s.lyraDir)
}

func (c *Checker) fixMissingPluginContent(s lyraContentState, contentSourcePath string) CheckResult {
	if contentSourcePath != "" && c.Fix {
		return c.overlayLyraContent(contentSourcePath, s.lyraDir)
	}

	if contentSourcePath != "" {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: fmt.Sprintf("top-level Content/ found but plugin content missing for: %s. "+
				"Found downloaded Lyra project at %s; "+
				"run 'ludus init --fix' to auto-overlay",
				strings.Join(s.missingPlugins, ", "), contentSourcePath)}
	}

	return CheckResult{Name: "Lyra Content", Passed: false,
		Message: fmt.Sprintf("top-level Content/ found but plugin content missing for: %s. "+
			"Copy the ENTIRE downloaded Lyra project over %s (overlay, not just Content/). "+
			"Each GameFeature plugin has its own Content/ directory required for cooking.",
			strings.Join(s.missingPlugins, ", "), s.lyraDir)}
}

// overlayLyraContent copies Lyra project content from srcPath into dstPath.
func (c *Checker) overlayLyraContent(srcPath, dstPath string) CheckResult {
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: fmt.Sprintf("content source path does not exist: %s", srcPath)}
	}

	fmt.Printf("  Overlaying Lyra content from %s to %s...\n", srcPath, dstPath)

	var copyErr error
	if runtime.GOOS == "windows" {
		copyErr = c.robocopyOverlay(srcPath, dstPath)
	} else {
		copyErr = c.cpOverlay(srcPath, dstPath)
	}

	if copyErr != nil {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: fmt.Sprintf("failed to overlay content: %v", copyErr)}
	}

	gameData := filepath.Join(dstPath, "Content", "DefaultGameData.uasset")
	if _, err := os.Stat(gameData); os.IsNotExist(err) {
		return CheckResult{Name: "Lyra Content", Passed: false,
			Message: fmt.Sprintf("overlay completed but Content/DefaultGameData.uasset still missing; "+
				"verify %s contains the correct Lyra project", srcPath)}
	}

	return CheckResult{Name: "Lyra Content", Passed: true,
		Message: fmt.Sprintf("overlaid from %s", srcPath)}
}

func (c *Checker) robocopyOverlay(srcPath, dstPath string) error {
	cmd := exec.Command("robocopy", srcPath, dstPath, "/E", "/NFL", "/NDL", "/NJH", "/NJS", "/NC", "/NS")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() < 8 {
				return nil
			}
		}
		return fmt.Errorf("robocopy failed: %w", err)
	}
	return nil
}

func (c *Checker) cpOverlay(srcPath, dstPath string) error {
	if !strings.HasSuffix(srcPath, "/") {
		srcPath += "/"
	}
	cmd := exec.Command("cp", "-a", srcPath+".", dstPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// discoverLyraContent searches common paths for a downloaded Lyra project.
func discoverLyraContent() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	candidates := []string{
		filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame"),
		filepath.Join(home, "Documents", "Unreal Projects", "Lyra Starter Game"),
	}

	if runtime.GOOS == "windows" {
		if oneDrive := os.Getenv("OneDrive"); oneDrive != "" {
			candidates = append(candidates,
				filepath.Join(oneDrive, "Documents", "Unreal Projects", "LyraStarterGame"),
				filepath.Join(oneDrive, "Documents", "Unreal Projects", "Lyra Starter Game"),
			)
		}
	}

	for _, path := range candidates {
		if isLyraProject(path) {
			return path
		}
	}

	// Glob for versioned directories
	matches, err := filepath.Glob(filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame*"))
	if err == nil {
		for _, match := range matches {
			if isLyraProject(match) {
				return match
			}
		}
	}

	return ""
}

func isLyraProject(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "Lyra.uproject")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "Content", "DefaultGameData.uasset")); err == nil {
		return true
	}
	return false
}
