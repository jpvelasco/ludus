package root

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/prereq"
	"github.com/spf13/cobra"
)

var fixFlag bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Validate prerequisites and configure the environment",
	Long: `Checks that all required tools and dependencies are installed and properly
configured. This includes:

  - Unreal Engine source access (Epic Games GitHub)
  - Build toolchain (clang, cross-compilation toolchain)
  - Docker and container runtime
  - AWS CLI and credentials
  - Sufficient disk space and memory

On Windows, also checks:
  - Visual Studio with required workloads and MSVC v14.38
  - BuildConfiguration.xml for UBT toolchain pinning
  - Windows SDK version and NNERuntimeORT patch status

Use --fix to auto-apply fixes where possible (e.g., create BuildConfiguration.xml,
patch NNERuntimeORT.Build.cs).`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&fixFlag, "fix", false, "auto-fix issues where possible")
}

func runInit(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, fixFlag, &globals.Cfg.Game)
	checker.Backend = globals.ResolveContainerBackend("")
	results := checker.RunAll()

	failed, warned := countCheckResults(results)

	if globals.JSONOutput {
		return reportInitJSON(results, failed)
	}

	fmt.Println("Validating prerequisites...")
	fmt.Println()

	for _, r := range results {
		marker := "[OK]  "
		if !r.Passed {
			marker = "[FAIL]"
		} else if r.Warning {
			marker = "[WARN]"
		}
		fmt.Printf("  %s %-24s %s\n", marker, r.Name, r.Message)
	}

	fmt.Println()
	if failed > 0 {
		if warned > 0 {
			fmt.Printf("%d warning(s)\n", warned)
		}
		return fmt.Errorf("%d prerequisite check(s) failed", failed)
	}
	if warned > 0 {
		fmt.Printf("All prerequisites passed (%d warning(s)).\n", warned)
	} else {
		fmt.Println("All prerequisites passed.")
	}
	fmt.Println("\nNext: ludus engine build")
	return nil
}

// reportInitJSON emits the check results for automation and then fails the
// command when any check failed, mirroring human-mode semantics: callers get
// the results AND a nonzero exit (#748).
func reportInitJSON(results []prereq.CheckResult, failed int) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return fmt.Errorf("encoding results: %w", err)
	}
	if failed > 0 {
		return fmt.Errorf("%d prerequisite check(s) failed", failed)
	}
	return nil
}

// countCheckResults counts failed and warned checks across the results.
func countCheckResults(results []prereq.CheckResult) (failed, warned int) {
	for _, r := range results {
		if !r.Passed {
			failed++
		} else if r.Warning {
			warned++
		}
	}
	return failed, warned
}
