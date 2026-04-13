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

// BuildGame builds a dedicated server inside WSL2 using RunUAT.
func BuildGame(ctx context.Context, w *WSL2, opts GameOptions) (*gamePkg.BuildResult, error) {
	start := time.Now()

	projectPath := w.ToWSLPath(opts.ProjectPath)
	outputDir := w.ToWSLPath(opts.OutputDir)

	// Build the RunUAT command.
	args := buildRunUATArgs(opts, projectPath, outputDir)

	// Set DDC environment if configured.
	var envPrefix string
	if opts.DDCMode == ddc.ModeLocal && opts.DDCPath != "" {
		envPrefix = fmt.Sprintf("export UE-LocalDataCachePath='%s' && ", opts.DDCPath)
	}

	script := fmt.Sprintf(
		"%scd '%s' && bash Engine/Build/BatchFiles/RunUAT.sh %s",
		envPrefix, opts.EnginePath, strings.Join(args, " "),
	)

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

// buildRunUATArgs constructs the RunUAT arguments for a server build.
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
		fmt.Sprintf("-project=%s", projectPath),
		fmt.Sprintf("-platform=%s", platform),
		"-server",
		"-noclient",
		"-build",
		"-cook",
		"-stage",
		"-pak",
		"-archive",
		fmt.Sprintf("-archivedirectory=%s", outputDir),
		fmt.Sprintf("-serverconfig=%s", serverConfig),
	}

	if opts.ServerTarget != "" {
		args = append(args, fmt.Sprintf("-target=%s", opts.ServerTarget))
	}
	if opts.ServerMap != "" {
		args = append(args, fmt.Sprintf("-map=%s", opts.ServerMap))
	}
	if opts.SkipCook {
		// Remove -cook and add -skipcook.
		var filtered []string
		for _, a := range args {
			if a != "-cook" {
				filtered = append(filtered, a)
			}
		}
		args = filtered
		args = append(args, "-skipcook")
	}
	if opts.MaxJobs > 0 {
		args = append(args, fmt.Sprintf("-MaxParallelActions=%d", opts.MaxJobs))
	}

	return args
}
