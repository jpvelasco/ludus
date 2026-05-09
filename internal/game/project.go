package game

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devrecon/ludus/internal/config"
)

// LocateProject finds the game project within the engine source tree.
func (b *Builder) LocateProject() (string, error) {
	if b.opts.ProjectPath != "" {
		if _, err := os.Stat(b.opts.ProjectPath); err != nil {
			return "", fmt.Errorf("configured project path not found: %s", b.opts.ProjectPath)
		}
		return b.opts.ProjectPath, nil
	}

	if b.opts.ProjectName == "Lyra" {
		candidate := filepath.Join(b.opts.EnginePath, "Samples", "Games", "Lyra", "Lyra.uproject")
		if _, err := os.Stat(candidate); err != nil {
			return "", fmt.Errorf("Lyra.uproject not found at %s (set game.projectPath in ludus.yaml)", candidate)
		}
		return candidate, nil
	}

	return "", fmt.Errorf("game.projectPath must be set in ludus.yaml for project %q", b.opts.ProjectName)
}

// PartialBuildHint checks for cooked content from a previous server build
// that could be reused with --skip-cook to avoid re-cooking (30-60 min).
// Returns empty string if no partial build is detected or --skip-cook is set.
func (b *Builder) PartialBuildHint() string {
	if b.opts.SkipCook {
		return ""
	}

	projectPath, err := b.LocateProject()
	if err != nil {
		return ""
	}

	return b.partialBuildHint(projectPath)
}

func (b *Builder) partialBuildHint(projectPath string) string {
	projectDir := filepath.Dir(projectPath)
	platformDir := config.ServerPlatformDir(config.NormalizeArch(b.opts.Arch))
	cookedDir := filepath.Join(projectDir, "Saved", "Cooked", platformDir)
	if !dirHasContent(cookedDir) {
		return ""
	}

	if b.serverBuildOutputExists(projectDir, platformDir) {
		return ""
	}

	return fmt.Sprintf("Previous cooked content found at %s\n"+
		"  To skip re-cooking (saves 30-60 min), re-run with: ludus game build --skip-cook", cookedDir)
}

func (b *Builder) serverBuildOutputExists(projectDir, platformDir string) bool {
	serverBin := filepath.Join(b.serverOutputDir(projectDir), platformDir, b.serverTargetName())
	_, err := os.Stat(serverBin)
	return err == nil
}

// dirHasContent checks if a directory exists and contains at least one entry.
func dirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}
