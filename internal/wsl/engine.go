package wsl

import (
	"context"
	"fmt"
	"time"

	engPkg "github.com/devrecon/ludus/internal/engine"
)

// EngineOptions configures an engine build inside WSL2.
type EngineOptions struct {
	SourcePath string // Windows path to UE source
	MaxJobs    int
	WSLNative  bool   // if true, use synced ext4 path
	Version    string // e.g. "5.7"
}

// BuildEngine compiles Unreal Engine from source inside a WSL2 distro.
func BuildEngine(ctx context.Context, w *WSL2, opts EngineOptions) (*engPkg.BuildResult, error) {
	start := time.Now()

	// Resolve engine path inside WSL2.
	var enginePath string
	if opts.WSLNative {
		enginePath = ResolveSyncTarget(opts.Version)
	} else {
		enginePath = w.ToWSLPath(opts.SourcePath)
	}
	if enginePath == "" {
		return nil, fmt.Errorf("engine path is empty")
	}

	// Expand $HOME for WSL2 native paths (e.g. "$HOME/ludus/engine/5.7" → absolute).
	expanded, err := w.ExpandHomePaths(ctx, enginePath)
	if err != nil {
		return nil, fmt.Errorf("resolving engine path: %w", err)
	}
	enginePath = expanded[0]

	// Ensure build dependencies are installed.
	fmt.Printf("Checking build dependencies in WSL2 distro %q...\n", w.Distro)
	if err := w.EnsureDeps(ctx); err != nil {
		return nil, fmt.Errorf("ensuring build dependencies: %w", err)
	}

	// Step 1: Setup.sh
	fmt.Println("  Running Setup.sh...")
	setupScript := fmt.Sprintf("cd %s && bash Setup.sh", shellQuote(enginePath))
	if err := w.RunBash(ctx, setupScript); err != nil {
		return nil, fmt.Errorf("Setup.sh failed: %w", err)
	}

	// Step 2: GenerateProjectFiles.sh
	fmt.Println("  Generating project files...")
	genScript := fmt.Sprintf("cd %s && bash GenerateProjectFiles.sh", shellQuote(enginePath))
	if err := w.RunBash(ctx, genScript); err != nil {
		return nil, fmt.Errorf("GenerateProjectFiles.sh failed: %w", err)
	}

	// Step 3: Compile
	jobs := opts.MaxJobs
	if jobs == 0 {
		jobs = 4
	}
	fmt.Printf("  Compiling engine with %d parallel job(s)...\n", jobs)

	compileScript := fmt.Sprintf(
		"cd %s && make -j%d ShaderCompileWorker && make -j%d UnrealEditor",
		shellQuote(enginePath), jobs, jobs,
	)
	if err := w.RunBash(ctx, compileScript); err != nil {
		return nil, fmt.Errorf("engine compilation failed: %w", err)
	}

	duration := time.Since(start).Seconds()
	fmt.Printf("  Engine build complete in %.0fs\n", duration)

	return &engPkg.BuildResult{
		Success:    true,
		EnginePath: enginePath,
		Duration:   duration,
	}, nil
}
