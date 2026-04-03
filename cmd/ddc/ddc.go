package ddc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/ddc"
	"github.com/devrecon/ludus/internal/dockerbuild"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/toolchain"
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

  ludus ddc status    Show DDC backend, path, and size
  ludus ddc clean     Delete all cached data
  ludus ddc prune     Remove old entries (default: >30 days)
  ludus ddc warmup    Pre-warm engine shaders with a minimal cook`,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show DDC backend, path, and size on disk",
	RunE:  runStatus,
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete all DDC content",
	RunE:  runClean,
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old DDC entries",
	RunE:  runPrune,
}

var warmupCmd = &cobra.Command{
	Use:   "warmup",
	Short: "Pre-warm engine-level DDC with a minimal cook",
	Long: `Runs a minimal, engine-focused cook to pre-populate the DDC with
common engine shaders, material templates, Lumen data, and texture
compression formats. This makes the first real project cook faster.`,
	RunE: runWarmup,
}

func init() {
	pruneCmd.Flags().IntVar(&pruneDays, "days", 30, "remove entries older than this many days")

	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(cleanCmd)
	Cmd.AddCommand(pruneCmd)
	Cmd.AddCommand(warmupCmd)
}

func resolveDDCPath() (string, error) {
	return ddc.ResolvePath(globals.Cfg.DDC.LocalPath)
}

func runStatus(cmd *cobra.Command, args []string) error {
	mode := globals.ResolveDDCMode()
	ddcPath, err := resolveDDCPath()
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

func runClean(cmd *cobra.Command, args []string) error {
	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	freed, err := ddc.Clean(ddcPath)
	if err != nil {
		return fmt.Errorf("cleaning DDC: %w", err)
	}

	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":     true,
			"bytes_freed": freed,
		})
	}

	if freed == 0 {
		fmt.Println("DDC is already empty.")
	} else {
		fmt.Printf("DDC cleaned: %s freed\n", formatSize(freed))
	}
	return nil
}

func runPrune(cmd *cobra.Command, args []string) error {
	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	freed, err := ddc.Prune(ddcPath, pruneDays)
	if err != nil {
		return fmt.Errorf("pruning DDC: %w", err)
	}

	if globals.JSONOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":      true,
			"bytes_freed":  freed,
			"max_age_days": pruneDays,
		})
	}

	if freed == 0 {
		fmt.Printf("No DDC entries older than %d days.\n", pruneDays)
	} else {
		fmt.Printf("DDC pruned: %s freed (entries older than %d days)\n", formatSize(freed), pruneDays)
	}
	return nil
}

func runWarmup(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	if cfg.Engine.Backend != "docker" && cfg.Engine.DockerImage == "" {
		return fmt.Errorf("DDC warmup requires Docker backend (set engine.backend: docker in ludus.yaml or use --backend docker)")
	}

	ddcMode := globals.ResolveDDCMode()
	if ddcMode == "none" {
		return fmt.Errorf("DDC warmup requires DDC mode 'local' (current mode: none)")
	}

	ddcPath, err := resolveDDCPath()
	if err != nil {
		return err
	}

	engineImage := cfg.Engine.DockerImage
	if engineImage == "" {
		imageName := cfg.Engine.DockerImageName
		if imageName == "" {
			imageName = "ludus-engine"
		}
		version, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)
		tag := version
		if tag == "" {
			tag = "latest"
		}
		engineImage = fmt.Sprintf("%s:%s", imageName, tag)
	}

	r := runner.NewRunner(globals.Verbose, globals.DryRun)
	builder := dockerbuild.NewDockerGameBuilder(dockerbuild.DockerGameOptions{
		EngineImage:   engineImage,
		ProjectPath:   cfg.Game.ProjectPath,
		ProjectName:   cfg.Game.ProjectName,
		ServerTarget:  cfg.Game.ResolvedServerTarget(),
		GameTarget:    cfg.Game.ResolvedGameTarget(),
		SkipCook:      false,
		ServerMap:     "",
		EngineVersion: cfg.Engine.Version,
		DDCMode:       ddcMode,
		DDCPath:       ddcPath,
	}, r)

	fmt.Println("DDC warmup: running minimal engine cook to populate shader cache...")
	_, err = builder.Build(context.Background())
	if err != nil {
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
