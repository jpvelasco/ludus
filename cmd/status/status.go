package status

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	internalstatus "github.com/devrecon/ludus/internal/status"
	"github.com/spf13/cobra"
)

// Cmd is the status command.
var Cmd = &cobra.Command{
	Use:   "status",
	Short: "Check status of all pipeline stages",
	Long: `Displays the current state of each pipeline stage:

  - Engine:    Is the engine source present? Built?
  - Game:      Is the server target compiled? Content cooked?
  - Container: Is the Docker image built? Pushed to ECR?
  - Deploy:    Is the target deployed? Active? Game sessions?`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	target, err := globals.ResolveTarget(cmd.Context(), cfg, "")
	if err != nil {
		// Non-fatal: status check should still report other stages
		fmt.Fprintf(os.Stderr, "Warning: could not resolve deploy target: %v\n", err)
	}

	stages := internalstatus.CheckAll(cmd.Context(), cfg, target)

	if globals.JSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stages)
	}

	fmt.Println("Pipeline Status")
	fmt.Println("===============")
	for _, s := range stages {
		marker := "[--]"
		switch s.Status {
		case "ok":
			marker = "[OK]"
		case "fail":
			marker = "[FAIL]"
		}
		line := fmt.Sprintf("  %s  %-24s", marker, s.Name)
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		fmt.Println(line)
	}
	fmt.Println()
	return nil
}
