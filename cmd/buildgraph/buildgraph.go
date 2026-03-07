package buildgraph

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/buildgraph"
	"github.com/devrecon/ludus/internal/toolchain"
	"github.com/spf13/cobra"
)

var (
	outputPath string
	toStdout   bool
)

// Cmd is the buildgraph command.
var Cmd = &cobra.Command{
	Use:   "buildgraph",
	Short: "Generate UE5 BuildGraph XML from ludus config",
	Long: `Generates a BuildGraph XML file describing the engine and game build pipeline
as a directed acyclic graph (DAG). The output is consumable by Horde, UET, or
any BuildGraph-compatible orchestrator.

By default writes to Build/BuildGraph.xml in the project directory.
Use --output/-o to specify a custom path, or --stdout to print to stdout.`,
	RunE: runBuildGraph,
}

func init() {
	Cmd.Flags().StringVarP(&outputPath, "output", "o", "", "output file path (default: Build/BuildGraph.xml in project dir)")
	Cmd.Flags().BoolVar(&toStdout, "stdout", false, "print XML to stdout instead of writing to file")
}

func runBuildGraph(_ *cobra.Command, _ []string) error {
	cfg := globals.Cfg

	engineVersion, _ := toolchain.DetectEngineVersion(cfg.Engine.SourcePath, cfg.Engine.Version)

	var opts []buildgraph.GenerateOption
	bg, err := buildgraph.Generate(cfg, engineVersion, opts...)
	if err != nil {
		return err
	}

	data, err := bg.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling BuildGraph XML: %w", err)
	}

	if toStdout {
		_, err = os.Stdout.Write(data)
		return err
	}

	outPath := outputPath
	if outPath == "" {
		projectDir := filepath.Dir(cfg.Game.ProjectPath)
		if projectDir == "." || projectDir == "" {
			projectDir = "."
		}
		outPath = filepath.Join(projectDir, "Build", "BuildGraph.xml")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	fmt.Printf("BuildGraph XML written to %s\n", outPath)
	return nil
}
