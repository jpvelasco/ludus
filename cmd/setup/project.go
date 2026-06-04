package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/ludus/internal/config"
)

// promptGameProjectDefault asks about game project configuration, using existing
// config values as defaults when provided.
func promptGameProjectDefault(enginePath, defaultName string, existing *config.Config) (projectName, projectPath, contentSourcePath string) {
	projectName = prompt("Project name", defaultName)

	if projectName == "Lyra" && enginePath != "" {
		contentSourcePath = promptLyraContent(enginePath)
	} else {
		projectPath = promptCustomProjectDefault(existingString("", existing, func(c *config.Config) string { return c.Game.ProjectPath }))
	}

	return projectName, projectPath, contentSourcePath
}

// promptCustomProjectDefault prompts for a .uproject path using defaultPath as the pre-fill.
func promptCustomProjectDefault(defaultPath string) string {
	projectPath := prompt("Path to .uproject file", defaultPath)
	if projectPath != "" {
		if _, err := os.Stat(projectPath); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		}
	}
	return projectPath
}

// promptLyraContent discovers or prompts for Lyra content source path.
func promptLyraContent(enginePath string) string {
	lyraDir := filepath.Join(enginePath, "Samples", "Games", "Lyra")
	uproject := filepath.Join(lyraDir, "Lyra.uproject")
	if _, err := os.Stat(uproject); err == nil {
		fmt.Printf("  Found Lyra at %s\n", lyraDir)
	}

	contentPath := discoverLyraContent()
	if contentPath != "" {
		fmt.Printf("  Found Lyra content download at %s\n", contentPath)
		if !confirm("  Use this as content source?") {
			contentPath = ""
		}
	}
	if contentPath == "" {
		contentPath = prompt("  Lyra content source path (or press Enter to skip)", "")
	}
	return contentPath
}

// discoverLyraContent scans common paths for downloaded Lyra content.
// Mirrors the logic in internal/prereq/checker.go.
func discoverLyraContent() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	for _, c := range lyraContentCandidates(home) {
		if isLyraProject(c) {
			return c
		}
	}

	return discoverVersionedLyraContent(home)
}

func lyraContentCandidates(home string) []string {
	candidates := []string{
		filepath.Join(home, "Documents", "Unreal Projects", "LyraStarterGame"),
		filepath.Join(home, "Documents", "Unreal Projects", "Lyra Starter Game"),
	}

	if runtime.GOOS != "windows" {
		return candidates
	}
	if oneDrive := os.Getenv("OneDrive"); oneDrive != "" {
		candidates = append(candidates,
			filepath.Join(oneDrive, "Documents", "Unreal Projects", "LyraStarterGame"),
			filepath.Join(oneDrive, "Documents", "Unreal Projects", "Lyra Starter Game"),
		)
	}
	return candidates
}

func discoverVersionedLyraContent(home string) string {
	docsDir := filepath.Join(home, "Documents", "Unreal Projects")
	matches, _ := filepath.Glob(filepath.Join(docsDir, "LyraStarterGame*"))
	for _, m := range matches {
		if isLyraProject(m) {
			return m
		}
	}
	return ""
}

// isLyraProject checks if a directory looks like a Lyra project download.
func isLyraProject(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "Lyra.uproject")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "Content", "DefaultGameData.uasset")); err == nil {
		return true
	}
	return false
}
