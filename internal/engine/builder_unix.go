//go:build !windows

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Setup runs Setup.sh to download engine dependencies.
func (b *Builder) Setup(ctx context.Context) error {
	setupPath := filepath.Join(b.opts.SourcePath, "Setup.sh")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		return fmt.Errorf("Setup.sh not found at %s", setupPath)
	}

	return b.Runner.RunInDir(ctx, b.opts.SourcePath, "bash", "Setup.sh")
}

// GenerateProjectFiles runs GenerateProjectFiles.sh.
func (b *Builder) GenerateProjectFiles(ctx context.Context) error {
	genPath := filepath.Join(b.opts.SourcePath, "GenerateProjectFiles.sh")
	if _, err := os.Stat(genPath); os.IsNotExist(err) {
		return fmt.Errorf("GenerateProjectFiles.sh not found at %s", genPath)
	}

	return b.Runner.RunInDir(ctx, b.opts.SourcePath, "bash", "GenerateProjectFiles.sh")
}

// compile builds ShaderCompileWorker and UnrealEditor using make.
func (b *Builder) compile(ctx context.Context, jobs int) error {
	jobsFlag := fmt.Sprintf("-j%d", jobs)
	targets := []string{"ShaderCompileWorker", "UnrealEditor"}

	for _, target := range targets {
		fmt.Printf("  Building %s...\n", target)
		if err := b.Runner.RunInDir(ctx, b.opts.SourcePath, "make", jobsFlag, target); err != nil {
			return fmt.Errorf("make %s failed: %w", target, err)
		}
	}
	return nil
}

// autoDetectJobs calculates the number of parallel compile jobs based on
// available RAM. UE5 linking can spike ~8GB per job.
func autoDetectJobs() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 1
	}
	defer f.Close()

	var memKB uint64
	if _, err := fmt.Fscanf(f, "MemTotal: %d kB", &memKB); err != nil || memKB == 0 {
		return 1
	}

	memGB := memKB / (1024 * 1024)
	jobs := max(int(memGB/8), 1)
	return jobs
}
