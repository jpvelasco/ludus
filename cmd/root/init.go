package root

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Validate prerequisites and configure the environment",
	Long: `Checks that all required tools and dependencies are installed and properly
configured. This includes:

  - Unreal Engine source access (Epic Games GitHub)
  - Build toolchain (clang, cross-compilation toolchain)
  - Docker and container runtime
  - AWS CLI and credentials
  - Sufficient disk space and memory`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath)
	results := checker.RunAll()

	if globals.JSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	fmt.Println("Validating prerequisites...")
	fmt.Println()

	failed := 0
	for _, r := range results {
		marker := "[OK]"
		if !r.Passed {
			marker = "[FAIL]"
			failed++
		}
		fmt.Printf("  %-6s %-20s %s\n", marker, r.Name, r.Message)
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d prerequisite check(s) failed", failed)
	}
	fmt.Println("All prerequisites passed.")
	return nil
}
