package ddc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/ddc"
	"github.com/jpvelasco/ludus/internal/dockerbuild"
	"github.com/spf13/cobra"
)

var pruneDays int

// Cmd is the top-level ddc command group.
var Cmd = &cobra.Command{
	Use:   "ddc",
	Short: "Manage the Derived Data Cache (DDC)",
	Long: `Commands for managing the UE5 Derived Data Cache.

The DDC stores pre-computed shader, texture, and asset data so that
subsequent builds skip expensive re-derivation.

  ludus ddc status    Show DDC mode, path, and size
  ludus ddc clean     Delete all cached data
  ludus ddc prune     Remove old entries (default: >30 days)
  ludus ddc warmup    Pre-warm the DDC with a cook-only build`,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show DDC mode, path, and size on disk",
	RunE:  runStatus,
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete all DDC content",
	Long: `Deletes all cached DDC content for the active backend.

In zen mode (the default) this resets the whole Zen store directory; the
store is rebuilt on the next build. In local mode it empties the legacy
FileSystem cache. With DDC disabled (mode none) there is nothing to clean.
Honors --dry-run with a preview that removes nothing.`,
	RunE: runClean,
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old DDC entries",
	Long: `Removes DDC entries older than --days (default: 30) from the legacy
FileSystem cache (mode: local).

Not supported in zen mode: the Zen store is an opaque key-value store, not a
per-entry file tree, so age-based pruning is not valid there. Run
'ludus ddc clean' to reset the whole Zen store instead.
Honors --dry-run with a preview that removes nothing.`,
	RunE: runPrune,
}

var warmupCmd = &cobra.Command{
	Use:   "warmup",
	Short: "Pre-warm the DDC with a cook-only container build",
	Long: `Runs a minimal cook-only container build to pre-populate the DDC with
engine-level shaders and base derived data. Uses MinimalDefaultMap to
minimize project content cooked. This makes subsequent full builds faster
by caching expensive shader compilations.

Flags passed to RunUAT: -cook -skipbuild -NoCompile -NoCompileEditor -NoP4 -map=MinimalDefaultMap`,
	RunE: runWarmup,
}

func init() {
	pruneCmd.Flags().IntVar(&pruneDays, "days", 30, "remove entries older than this many days")

	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(cleanCmd)
	Cmd.AddCommand(pruneCmd)
	Cmd.AddCommand(warmupCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Report the path for the active backend: zen mode persists the ZenStore
	// directory, local mode the legacy FileSystem cache. "none" has no path.
	mode, ddcPath, err := globals.ResolveActiveDDC()
	if err != nil {
		return err
	}

	size, err := ddc.DirSize(ddcPath)
	if err != nil {
		return fmt.Errorf("calculating DDC size: %w", err)
	}

	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"mode":       mode,
			"path":       ddcPath,
			"size_bytes": size,
		})
	}

	fmt.Printf("DDC Status\n")
	fmt.Printf("  Mode: %s\n", mode)
	fmt.Printf("  Path: %s\n", ddcPath)
	fmt.Printf("  Size: %s\n", formatSize(size))
	return nil
}

// warmupPath returns the cache path the warmup cook will populate for the
// active mode: the ZenStore dir for zen, the FileSystem dir for local.
func warmupPath(mode, ddcPath, ddcZenPath string) string {
	if mode == ddc.ModeZen {
		return ddcZenPath
	}
	return ddcPath
}

// zenPruneUnsupported is surfaced when prune is requested while the Zen
// backend is active: the Zen store is an opaque key-value store rather than
// a per-entry file tree, so removing "old" files would corrupt a live store.
// clean remains the supported way to reset the whole Zen store.
const zenPruneUnsupported = "ddc prune is not supported for the Zen DDC store: the Zen store is an " +
	"opaque key-value store, not a per-entry file tree, so age-based pruning is not valid there. " +
	"Run 'ludus ddc clean' to reset the whole Zen store"

func runClean(cmd *cobra.Command, args []string) error {
	mode, ddcPath, err := globals.ResolveActiveDDC()
	if err != nil {
		return err
	}
	if mode == ddc.ModeNone {
		return reportDDCDisabled("clean")
	}
	if globals.DryRun {
		return previewClean(ddcPath)
	}

	freed, err := ddc.Clean(ddcPath)
	if err != nil {
		return fmt.Errorf("cleaning DDC: %w", err)
	}
	return reportCleanFreed(freed)
}

func runPrune(cmd *cobra.Command, args []string) error {
	mode, ddcPath, err := globals.ResolveActiveDDC()
	if err != nil {
		return err
	}
	if mode == ddc.ModeNone {
		return reportDDCDisabled("prune")
	}
	if mode == ddc.ModeZen {
		return errors.New(zenPruneUnsupported)
	}
	if globals.DryRun {
		return previewPrune(ddcPath)
	}

	freed, err := ddc.Prune(ddcPath, pruneDays)
	if err != nil {
		return fmt.Errorf("pruning DDC: %w", err)
	}
	return reportPruneFreed(freed)
}

// reportDDCDisabled reports the nothing-to-do outcome for a disabled DDC.
func reportDDCDisabled(operation string) error {
	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":     true,
			"bytes_freed": 0,
		})
	}
	fmt.Printf("DDC is disabled; nothing to %s.\n", operation)
	return nil
}

// previewClean reports what clean would free without touching the cache,
// backing the --dry-run gate in front of the destructive operation (#700).
func previewClean(ddcPath string) error {
	size, err := ddc.DirSize(ddcPath)
	if err != nil {
		return fmt.Errorf("calculating DDC size: %w", err)
	}
	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":     true,
			"dry_run":     true,
			"bytes_freed": size,
		})
	}
	fmt.Println("DRY RUN: DDC clean")
	fmt.Printf("  Path      : %s\n", ddcPath)
	fmt.Printf("  Would free: %s\n", formatSize(size))
	return nil
}

// previewPrune reports what prune would free without removing anything.
func previewPrune(ddcPath string) error {
	freed, err := ddc.PrunePreview(ddcPath, pruneDays)
	if err != nil {
		return fmt.Errorf("pruning DDC: %w", err)
	}
	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":      true,
			"dry_run":      true,
			"bytes_freed":  freed,
			"max_age_days": pruneDays,
		})
	}
	fmt.Println("DRY RUN: DDC prune")
	fmt.Printf("  Path       : %s\n", ddcPath)
	fmt.Printf("  Max age    : %d days\n", pruneDays)
	fmt.Printf("  Would free : %s\n", formatSize(freed))
	return nil
}

// reportCleanFreed reports the bytes clean removed.
func reportCleanFreed(freed int64) error {
	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":     true,
			"bytes_freed": freed,
		})
	}
	if freed == 0 {
		fmt.Println("DDC is already empty.")
		return nil
	}
	fmt.Printf("DDC cleaned: %s freed\n", formatSize(freed))
	return nil
}

// reportPruneFreed reports the bytes prune removed.
func reportPruneFreed(freed int64) error {
	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":      true,
			"bytes_freed":  freed,
			"max_age_days": pruneDays,
		})
	}
	if freed == 0 {
		fmt.Printf("No DDC entries older than %d days.\n", pruneDays)
		return nil
	}
	fmt.Printf("DDC pruned: %s freed (entries older than %d days)\n", formatSize(freed), pruneDays)
	return nil
}

func runWarmup(cmd *cobra.Command, args []string) error {
	ddcMode, ddcPath, ddcZenPath, err := globals.ResolveDDC()
	if err != nil {
		return err
	}
	if ddcMode == ddc.ModeNone {
		return fmt.Errorf("DDC warmup requires DDC to be enabled; set ddc.mode to %q or %q (current: %q)", ddc.ModeZen, ddc.ModeLocal, ddcMode)
	}

	if globals.DryRun {
		return printWarmupPreview(warmupPath(ddcMode, ddcPath, ddcZenPath))
	}

	return executeWarmup(cmd.Context(), ddcMode, ddcPath, ddcZenPath)
}

func printWarmupPreview(ddcPath string) error {
	cfg := globals.Cfg
	engineImage, err := globals.ResolveEngineImage(cfg, true)
	if err != nil {
		return err
	}
	fmt.Println("DRY RUN: DDC Warmup")
	fmt.Printf("  Image  : %s\n", engineImage)
	fmt.Printf("  Project: %s\n", cfg.Game.ProjectPath)
	fmt.Printf("  DDC    : %s\n", ddcPath)
	fmt.Println("  Flags  : -cook -skipbuild -NoCompile -NoCompileEditor -NoP4 -map=MinimalDefaultMap")
	return nil
}

func executeWarmup(ctx context.Context, ddcMode, ddcPath, ddcZenPath string) error {
	cfg := globals.Cfg

	if !dockerbuild.IsContainerBackend(cfg.Engine.Backend) && cfg.Engine.DockerImage == "" {
		return fmt.Errorf("DDC warmup requires a container backend (set engine.backend to podman or docker in ludus.yaml)")
	}

	engineImage, err := globals.ResolveEngineImage(cfg, true)
	if err != nil {
		return err
	}

	r := globals.NewRunner()
	opts := globals.BaseDockerGameOptions(cfg, engineImage, cfg.Engine.Version, ddcMode, ddcPath, ddcZenPath, cfg.Engine.Backend)
	opts.CookOnly = true
	builder := dockerbuild.NewDockerGameBuilder(opts, r)

	fmt.Println("DDC warmup: running cook-only build to populate shader cache...")
	if _, err := builder.Build(ctx); err != nil {
		return fmt.Errorf("DDC warmup failed: %w", err)
	}

	fmt.Println("DDC warmup complete.")
	return nil
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
