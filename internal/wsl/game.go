package wsl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devrecon/ludus/internal/ddc"
	gamePkg "github.com/devrecon/ludus/internal/game"
)

// GameOptions configures a game server build inside WSL2.
type GameOptions struct {
	EnginePath   string // WSL path to engine
	ProjectPath  string // Windows path to .uproject
	ProjectName  string
	ServerTarget string
	Platform     string
	Arch         string
	SkipCook     bool
	ServerMap    string
	OutputDir    string // Windows path for output
	DDCMode      string
	DDCPath      string // WSL path to DDC dir
	ServerConfig string
	MaxJobs      int
}

// validateBuildGameOpts checks required fields before starting a game build.
// Extracted so it can be unit-tested without a live WSL2 connection.
func validateBuildGameOpts(opts GameOptions) error {
	if opts.EnginePath == "" {
		return fmt.Errorf("WSL2 engine path is empty; run: ludus engine build --backend wsl2")
	}
	if opts.DDCMode == ddc.ModeLocal && opts.DDCPath == "" {
		return fmt.Errorf("DDC mode is %q but DDC path is empty; re-run engine build or set ddc.path in ludus.yaml", ddc.ModeLocal)
	}
	return nil
}

// BuildGame builds a dedicated server inside WSL2 using RunUAT.
func BuildGame(ctx context.Context, w *WSL2, opts GameOptions) (*gamePkg.BuildResult, error) {
	start := time.Now()

	if err := validateBuildGameOpts(opts); err != nil {
		return nil, err
	}

	// Ensure runtime libraries are present — UnrealEditor-Cmd needs libnss3,
	// libdbus, etc. even in headless/server mode during the cook step.
	fmt.Println("Checking runtime dependencies...")
	if err := w.EnsureRuntimeDeps(ctx); err != nil {
		return nil, fmt.Errorf("ensuring runtime dependencies: %w", err)
	}

	// Expand $HOME in WSL2-native paths to absolute paths so all paths can be
	// safely single-quoted by shellQuote without relying on bash variable expansion.
	expanded, err := w.ExpandHomePaths(ctx, opts.EnginePath, opts.DDCPath)
	if err != nil {
		return nil, err
	}
	opts.EnginePath, opts.DDCPath = expanded[0], expanded[1]

	projectPath := w.ToWSLPath(opts.ProjectPath)
	outputDir := w.ToWSLPath(opts.OutputDir)

	script := buildGameScript(opts, projectPath, outputDir)

	fmt.Printf("Building game server in WSL2...\n")
	if err := w.RunBash(ctx, script); err != nil {
		return nil, fmt.Errorf("game build failed: %w", err)
	}

	duration := time.Since(start).Seconds()
	return &gamePkg.BuildResult{
		Success:   true,
		OutputDir: outputDir,
		Duration:  duration,
	}, nil
}

// buildGameScript constructs the bash script for running UAT inside WSL2.
// The script runs via: wsl.exe -d <distro> bash -c "<script>".
// All paths must be pre-resolved (no $HOME) before calling this function;
// shellQuote handles spaces and special characters in all path values.
func buildGameScript(opts GameOptions, projectPath, outputDir string) string {
	args := buildRunUATArgs(opts, projectPath, outputDir)

	var envPrefix string
	if opts.DDCMode == ddc.ModeLocal && opts.DDCPath != "" {
		// UE-LocalDataCachePath is not a valid shell identifier (hyphens are illegal
		// in any shell assignment). Use `env KEY=VALUE` to pass it to the child
		// process without shell assignment.
		envPrefix = fmt.Sprintf("env %s ", shellQuote(ddc.EnvOverride(opts.DDCPath)))
	}

	return fmt.Sprintf(
		"cd %s && %sbash Engine/Build/BatchFiles/RunUAT.sh %s",
		shellQuote(opts.EnginePath), envPrefix, strings.Join(args, " "),
	)
}

// buildRunUATArgs constructs the RunUAT arguments for a server build.
// Path values are shell-quoted so bash does not word-split on spaces
// (e.g. "/mnt/f/Source Code/..." → preserved as a single argument).
func buildRunUATArgs(opts GameOptions, projectPath, outputDir string) []string {
	platform := opts.Platform
	if platform == "" {
		platform = "Linux"
	}

	serverConfig := opts.ServerConfig
	if serverConfig == "" {
		serverConfig = "Development"
	}

	args := []string{
		"BuildCookRun",
		fmt.Sprintf("-project=%s", shellQuote(projectPath)),
		fmt.Sprintf("-platform=%s", platform),
		"-server",
		"-noclient",
		"-build",
		"-cook",
		"-stage",
		"-pak",
		"-archive",
		fmt.Sprintf("-archivedirectory=%s", shellQuote(outputDir)),
		fmt.Sprintf("-serverconfig=%s", serverConfig),
	}

	if opts.ServerTarget != "" {
		args = append(args, fmt.Sprintf("-target=%s", opts.ServerTarget))
	}
	if opts.ServerMap != "" {
		args = append(args, fmt.Sprintf("-map=%s", opts.ServerMap))
	}
	if opts.SkipCook {
		args = applySkipCook(args)
	}
	if opts.MaxJobs > 0 {
		args = append(args, fmt.Sprintf("-MaxParallelActions=%d", opts.MaxJobs))
	}

	return args
}

// applySkipCook removes -cook from args and appends -skipcook.
func applySkipCook(args []string) []string {
	var filtered []string
	for _, a := range args {
		if a != "-cook" {
			filtered = append(filtered, a)
		}
	}
	return append(filtered, "-skipcook")
}
