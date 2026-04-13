package wsl

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/runner"
)

const (
	// NativeEngineBase is the base directory for engine source on native ext4.
	NativeEngineBase = "~/ludus/engine"
	// NativeDDCDir is the DDC cache directory on native ext4.
	NativeDDCDir = "~/ludus/ddc"
	// MinDiskSpaceGB is the minimum free disk space required for a native sync.
	MinDiskSpaceGB = 120.0
)

// SyncOptions configures the engine source sync to native ext4.
type SyncOptions struct {
	SourcePath string // Windows path to UE source
	TargetDir  string // WSL2 target (default: ~/ludus/engine/<version>/)
	Version    string // Engine version (e.g. "5.7")
}

// SyncResult holds the outcome of a source sync operation.
type SyncResult struct {
	WSLPath  string
	DDCPath  string
	Duration time.Duration
}

// SyncEngine rsyncs engine source from Windows to native ext4 in WSL2.
// This is the high-performance path for --wsl-native.
func SyncEngine(ctx context.Context, r *runner.Runner, distro string, opts SyncOptions) (*SyncResult, error) {
	start := time.Now()

	targetDir := opts.TargetDir
	if targetDir == "" {
		if opts.Version == "" {
			targetDir = NativeEngineBase + "/default"
		} else {
			targetDir = NativeEngineBase + "/" + opts.Version
		}
	}

	// Check disk space.
	freeGB, err := CheckDiskSpace(ctx, r, distro)
	if err != nil {
		return nil, fmt.Errorf("checking disk space: %w", err)
	}
	if freeGB < MinDiskSpaceGB {
		return nil, fmt.Errorf("insufficient disk space: %.0f GB free, need at least %.0f GB\n"+
			"Free up space in WSL2 or expand the virtual disk",
			freeGB, MinDiskSpaceGB)
	}

	// Create target directories.
	mkdirScript := fmt.Sprintf("mkdir -p %s && mkdir -p %s", targetDir, NativeDDCDir)
	if err := RunBash(ctx, r, distro, mkdirScript); err != nil {
		return nil, fmt.Errorf("creating directories: %w", err)
	}

	// Convert Windows source path to WSL mount path for rsync source.
	wslSource := ToWSLPath(opts.SourcePath)
	if wslSource == "" {
		return nil, fmt.Errorf("empty source path")
	}

	// Rsync engine source to native ext4.
	// --delete ensures the target mirrors the source exactly.
	// --info=progress2 gives a compact progress summary.
	rsyncCmd := fmt.Sprintf(
		"rsync -a --info=progress2 --delete '%s/' '%s/'",
		wslSource, targetDir,
	)
	if err := RunBash(ctx, r, distro, rsyncCmd); err != nil {
		return nil, fmt.Errorf("rsync failed: %w", err)
	}

	return &SyncResult{
		WSLPath:  targetDir,
		DDCPath:  NativeDDCDir,
		Duration: time.Since(start),
	}, nil
}

// NeedsResync returns true if the engine source should be re-synced.
// Re-sync is needed if no previous sync exists or if the source is newer.
func NeedsResync(lastSyncTime time.Time) bool {
	if lastSyncTime.IsZero() {
		return true
	}
	// Re-sync if last sync was more than 24 hours ago.
	return time.Since(lastSyncTime) > 24*time.Hour
}

// ResolveSyncTarget returns the target directory for the engine sync.
func ResolveSyncTarget(version string) string {
	if version == "" {
		return NativeEngineBase + "/default"
	}
	return NativeEngineBase + "/" + version
}
