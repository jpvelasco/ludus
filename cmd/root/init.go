package root

import (
	"fmt"

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
	fmt.Println("Validating prerequisites...")

	// TODO: Wire up prereq.Checker
	checks := []string{
		"Unreal Engine source directory",
		"Build toolchain (clang)",
		"Docker",
		"AWS CLI",
		"AWS credentials",
		"Disk space",
		"Memory",
	}

	for _, check := range checks {
		fmt.Printf("  [--] %s\n", check)
	}

	fmt.Println("\nInit not yet implemented. This will validate your environment.")
	return nil
}
