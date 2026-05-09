package setup

import (
	"github.com/spf13/cobra"
)

// Cmd is the setup wizard command.
var Cmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard for first-time configuration",
	Long: `Guided setup that scans your system, auto-detects settings, and writes
a complete ludus.yaml configuration file.

Steps:
  1. Locate Unreal Engine source directory
  2. Auto-detect engine version from Build.version
  3. Configure game project (Lyra or custom)
  4. Choose deployment target
  5. Configure AWS settings (optional)
  6. Write ludus.yaml

Use --profile to create a profile-specific config (ludus-<profile>.yaml).`,
	RunE: runSetup,
}
