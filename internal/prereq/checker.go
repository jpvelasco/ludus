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
	// Docker is in PATH — verify the daemon is running.
	if err := exec.Command("docker", "info").Run(); err != nil {
		return CheckResult{
			Name:    "Docker",
			Passed:  false,
			Message: "docker found but daemon is not running; start Docker Desktop or the docker service",
		}
	}
	return CheckResult{
		Name:    "Docker",
		Passed:  true,
		Message: "docker daemon running",
	}
}

// checkCrossArchEmulation verifies that Docker can build for the target
// architecture when it differs from the host. Cross-architecture builds
// (e.g. arm64 on an amd64 host) require QEMU user-mode emulation via binfmt_misc.
func (c *Checker) checkCrossArchEmulation() CheckResult {
	name := "Cross-Arch Emulation"

	if c.GameConfig == nil {
		return CheckResult{Name: name, Passed: true, Message: "no game config; skipping"}
	}

	targetArch := c.GameConfig.ResolvedArch()
	if targetArch == runtime.GOARCH {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("native build (%s); no emulation needed", targetArch),
		}
	}

	// Docker must be available for this check to be meaningful.
	if _, err := exec.LookPath("docker"); err != nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "docker not found; skipping cross-arch check",
		}
	}

	// Map Go arch names to Docker platform strings.
	dockerPlatform := "linux/" + targetArch

	out, err := exec.Command("docker", "buildx", "inspect", "--bootstrap").CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Warning: true,
			Message: "docker buildx not available; cannot verify cross-arch support",
		}
	}

	if strings.Contains(string(out), dockerPlatform) {
		return CheckResult{
			Name:    name,
			Passed:  true,
			Message: fmt.Sprintf("Docker can build for %s (QEMU emulation registered)", dockerPlatform),
		}
	}

	return CheckResult{
		Name:   name,
		Passed: false,
		Message: fmt.Sprintf("Docker cannot build for %s on this %s host; "+
			"install QEMU emulation with:\n"+
			"    docker run --rm --privileged tonistiigi/binfmt --install %s",
			dockerPlatform, runtime.GOARCH, targetArch),
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
	var foundPath string
	_ = filepath.WalkDir(contentDir, func(path string, d fs.DirEntry, err error) error {
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
		rel, _ := filepath.Rel(contentDir, foundPath)
		return CheckResult{Name: "Server Map", Passed: true,
			Message: fmt.Sprintf("'%s' found at Content/%s", serverMap, rel)}
	}
	return CheckResult{Name: "Server Map", Passed: true, Warning: true,
		Message: fmt.Sprintf("'%s.umap' not found under %s; verify serverMap in ludus.yaml", serverMap, contentDir)}
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
