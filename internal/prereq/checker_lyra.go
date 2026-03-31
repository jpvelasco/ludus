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

	contentMissing := false
	if _, err := os.Stat(gameData); os.IsNotExist(err) {
		contentMissing = true
	}

	// Verify plugin content dirs exist (common oversight: copying only top-level Content/)
	pluginContentDirs := []string{
		"ShooterCore",
		"ShooterMaps",
		"TopDownArena",
	}

	var missingPlugins []string
	if !contentMissing {
		for _, plugin := range pluginContentDirs {
			pluginDir := filepath.Join(lyraDir, "Plugins", "GameFeatures", plugin, "Content")
			if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
				missingPlugins = append(missingPlugins, plugin)
			}
		}
	}

	// Content is fully present
	if !contentMissing && len(missingPlugins) == 0 {
		return CheckResult{
			Name:    "Lyra Content",
			Passed:  true,
			Message: fmt.Sprintf("found at %s (including plugin content)", contentDir),
		}
	}

	// Content is missing — check if we can auto-fix
	contentSourcePath := ""
	if c.GameConfig != nil {
		contentSourcePath = c.GameConfig.ContentSourcePath
	}

	if contentMissing {
		if contentSourcePath == "" {
			// Try auto-discovery of downloaded Lyra content
			discovered := discoverLyraContent()
			if discovered != "" {
				if c.Fix {
					return c.overlayLyraContent(discovered, lyraDir)
				}
				return CheckResult{
					Name:   "Lyra Content",
					Passed: false,
					Message: fmt.Sprintf("Lyra Content not found at %s. "+
						"Found downloaded Lyra project at %s; "+
						"run 'ludus init --fix' to auto-overlay, "+
						"or set game.contentSourcePath in ludus.yaml",
						contentDir, discovered),
				}
			}
			return CheckResult{
				Name:   "Lyra Content",
				Passed: false,
				Message: fmt.Sprintf("Lyra Content not found at %s. "+
					"Download 'Lyra Starter Game' from the Epic Games Launcher, "+
					"then either copy it to %s manually, or set game.contentSourcePath "+
					"in ludus.yaml and run ludus init --fix", contentDir, lyraDir),
			}
		}
		if !c.Fix {
			return CheckResult{
				Name:   "Lyra Content",
				Passed: false,
				Message: fmt.Sprintf("Lyra Content not found at %s; "+
					"run with --fix to overlay from %s",
					contentDir, contentSourcePath),
			}
		}
		return c.overlayLyraContent(contentSourcePath, lyraDir)
	}

	// Top-level content exists but plugin content is missing
	if contentSourcePath != "" && c.Fix {
		return c.overlayLyraContent(contentSourcePath, lyraDir)
	}

	// Try auto-discovery for plugin content fix
	if contentSourcePath == "" {
		discovered := discoverLyraContent()
		if discovered != "" {
			if c.Fix {
				return c.overlayLyraContent(discovered, lyraDir)
			}
			return CheckResult{
				Name:   "Lyra Content",
				Passed: false,
				Message: fmt.Sprintf("top-level Content/ found but plugin content missing for: %s. "+
					"Found downloaded Lyra project at %s; "+
					"run 'ludus init --fix' to auto-overlay",
					strings.Join(missingPlugins, ", "), discovered),
			}
		}
	}

	return CheckResult{
		Name:   "Lyra Content",
		Passed: false,
		Message: fmt.Sprintf("top-level Content/ found but plugin content missing for: %s. "+
			"Copy the ENTIRE downloaded Lyra project over %s (overlay, not just Content/). "+
			"Each GameFeature plugin has its own Content/ directory required for cooking.",
			strings.Join(missingPlugins, ", "), lyraDir),
	}
}

// overlayLyraContent copies the downloaded Lyra project content from
// contentSourcePath into the engine's Lyra directory. This overlays Content/
// directories at both the top level and under Plugins/GameFeatures/*/Content/.
func (c *Checker) overlayLyraContent(srcPath, dstPath string) CheckResult {
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return CheckResult{
			Name:    "Lyra Content",
			Passed:  false,
			Message: fmt.Sprintf("content source path does not exist: %s", srcPath),
		}
	}

	fmt.Printf("  Overlaying Lyra content from %s to %s...\n", srcPath, dstPath)

	// Use robocopy on Windows (handles long paths, preserves structure) or
	// cp -a on Unix. We copy the entire source directory contents into the
	// destination, which overlays Content/ and Plugins/ without destroying
	// existing source code files.
	var copyErr error
	if runtime.GOOS == "windows" {
		copyErr = c.robocopyOverlay(srcPath, dstPath)
	} else {
		copyErr = c.cpOverlay(srcPath, dstPath)
	}

	if copyErr != nil {
		return CheckResult{
			Name:    "Lyra Content",
			Passed:  false,
			Message: fmt.Sprintf("failed to overlay content: %v", copyErr),
		}
	}

	// Verify the overlay worked
	gameData := filepath.Join(dstPath, "Content", "DefaultGameData.uasset")
	if _, err := os.Stat(gameData); os.IsNotExist(err) {
		return CheckResult{
			Name:   "Lyra Content",
			Passed: false,
			Message: fmt.Sprintf("overlay completed but Content/DefaultGameData.uasset still missing; "+
				"verify %s contains the correct Lyra project", srcPath),
		}
	}

	return CheckResult{
		Name:    "Lyra Content",
		Passed:  true,
		Message: fmt.Sprintf("overlaid from %s", srcPath),
	}
}

// robocopyOverlay uses robocopy to copy srcPath contents into dstPath.
// Robocopy exit codes 0-7 indicate success (various levels of files copied/skipped).
func (c *Checker) robocopyOverlay(srcPath, dstPath string) error {
	cmd := exec.Command("robocopy", srcPath, dstPath, "/E", "/NFL", "/NDL", "/NJH", "/NJS", "/NC", "/NS")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		// robocopy returns non-zero for success (1 = files copied, etc.)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() < 8 {
				return nil // exit codes 0-7 are success
			}
		}
		return fmt.Errorf("robocopy failed: %w", err)
	}
	return nil
}

// cpOverlay uses cp -a to copy srcPath contents into dstPath.
func (c *Checker) cpOverlay(srcPath, dstPath string) error {
	// Ensure trailing slash on src so cp copies contents, not the directory itself
	if !strings.HasSuffix(srcPath, "/") {
		srcPath += "/"
	}
	cmd := exec.Command("cp", "-a", srcPath+".", dstPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// discoverLyraContent searches common paths for a downloaded Lyra Starter Game
// project. Returns the first valid path found, or "".
func discoverLyraContent() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Fixed candidate paths
	candidates := []string{
		filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame"),
		filepath.Join(home, "Documents", "Unreal Projects", "Lyra Starter Game"),
	}

	// On Windows, also check OneDrive-redirected Documents
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

	// Glob for versioned directories (e.g. LyraStarterGame_5.5)
	patterns := []string{
		filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame*"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			if isLyraProject(match) {
				return match
			}
		}
	}

	return ""
}

// isLyraProject checks whether the given directory looks like a downloaded
// Lyra Starter Game project (contains Lyra.uproject or Lyra-specific content).
func isLyraProject(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "Lyra.uproject")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "Content", "DefaultGameData.uasset")); err == nil {
		return true
	}
	return false
}
