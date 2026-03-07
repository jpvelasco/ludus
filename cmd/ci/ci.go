package ci

import (
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	internalci "github.com/devrecon/ludus/internal/ci"
	"github.com/spf13/cobra"
)

var (
	outputPath string
	enablePush bool
	enablePR   bool
)

// Cmd is the top-level ci command group.
var Cmd = &cobra.Command{
	Use:   "ci",
	Short: "CI workflow generation and runner management",
	Long: `Commands for generating GitHub Actions workflows and managing
self-hosted runner agents for the UE5 server pipeline.

  ludus ci init           Generate a GitHub Actions workflow file
  ludus ci runner         Manage the self-hosted runner agent`,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a GitHub Actions workflow file",
	Long: `Generates a GitHub Actions workflow file configured for the UE5 server pipeline.

The workflow uses workflow_dispatch for manual triggering with skip flags
for each pipeline stage. Push and PR triggers are commented out by default
(use --enable-push / --enable-pr to uncomment them).

The workflow assumes:
  - A self-hosted runner is registered and running
  - ludus.yaml exists on the runner machine (gitignored)
  - UE5 engine is already built on the runner (skip-engine defaults to true)`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&outputPath, "output", "o", "", "output path (default: from config or .github/workflows/ludus-pipeline.yml)")
	initCmd.Flags().BoolVar(&enablePush, "enable-push", false, "uncomment push trigger in workflow")
	initCmd.Flags().BoolVar(&enablePR, "enable-pr", false, "uncomment pull_request trigger in workflow")

	Cmd.AddCommand(initCmd)
	Cmd.AddCommand(runnerCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	path := outputPath
	if path == "" {
		path = cfg.CI.WorkflowPath
	}

	labels := cfg.CI.RunnerLabels
	if len(labels) == 0 {
		labels = []string{"self-hosted", "linux", "x64"}
	}

	content := internalci.GenerateWorkflow(internalci.WorkflowOptions{
		RunnerLabels: labels,
		EnablePush:   enablePush,
		EnablePR:     enablePR,
	})

	if globals.DryRun {
		fmt.Println(content)
		return nil
	}

	if err := internalci.WriteWorkflow(path, content); err != nil {
		return err
	}

	fmt.Printf("Workflow written to %s\n", path)
	return nil
}
