package pipeline

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	skipEngine    bool
	skipLyra      bool
	skipContainer bool
	skipDeploy    bool
	dryRun        bool
)

// Cmd is the full pipeline command.
var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full pipeline end-to-end",
	Long: `Executes the complete Ludus pipeline:

  1. Validate prerequisites (ludus init)
  2. Build Unreal Engine from source (ludus engine build)
  3. Integrate GameLift SDK into Lyra (ludus lyra integrate-gamelift)
  4. Build Lyra dedicated server for Linux (ludus lyra build)
  5. Build Docker container image (ludus container build)
  6. Push to Amazon ECR (ludus container push)
  7. Deploy to GameLift Containers (ludus deploy fleet)

Use --skip-* flags to skip stages that are already complete.
Use --dry-run to see what would be executed without running anything.`,
	RunE: runPipeline,
}

func init() {
	Cmd.Flags().BoolVar(&skipEngine, "skip-engine", false, "skip engine build (use existing build)")
	Cmd.Flags().BoolVar(&skipLyra, "skip-lyra", false, "skip Lyra build (use existing build)")
	Cmd.Flags().BoolVar(&skipContainer, "skip-container", false, "skip container build (use existing image)")
	Cmd.Flags().BoolVar(&skipDeploy, "skip-deploy", false, "skip deployment (build only)")
	Cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be executed without running")
}

func runPipeline(cmd *cobra.Command, args []string) error {
	steps := []struct {
		name    string
		skipped bool
	}{
		{"Validate prerequisites", false},
		{"Build Unreal Engine", skipEngine},
		{"Integrate GameLift SDK", skipLyra},
		{"Build Lyra server (Linux)", skipLyra},
		{"Build container image", skipContainer},
		{"Push to Amazon ECR", skipContainer},
		{"Deploy to GameLift", skipDeploy},
	}

	if dryRun {
		fmt.Println("Dry run — would execute:")
	} else {
		fmt.Println("Pipeline execution not yet implemented. Steps:")
	}

	for i, step := range steps {
		marker := ">>>"
		if step.skipped {
			marker = "---"
		}
		fmt.Printf("  %s %d. %s", marker, i+1, step.name)
		if step.skipped {
			fmt.Print(" (skipped)")
		}
		fmt.Println()
	}

	return nil
}
