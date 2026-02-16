package status

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Cmd is the status command.
var Cmd = &cobra.Command{
	Use:   "status",
	Short: "Check status of all pipeline stages",
	Long: `Displays the current state of each pipeline stage:

  - Engine:    Is the engine source present? Built?
  - Lyra:      Is the server target compiled? Content cooked?
  - Container: Is the Docker image built? Pushed to ECR?
  - GameLift:  Is the fleet deployed? Active? Game sessions?`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	stages := []struct {
		name   string
		status string
	}{
		{"Engine Source", "not checked"},
		{"Engine Build", "not checked"},
		{"Lyra Server Build", "not checked"},
		{"GameLift SDK Integration", "not checked"},
		{"Container Image", "not checked"},
		{"ECR Push", "not checked"},
		{"GameLift Fleet", "not checked"},
	}

	fmt.Println("Pipeline Status")
	fmt.Println("===============")
	for _, s := range stages {
		fmt.Printf("  %-28s [%s]\n", s.name, s.status)
	}
	fmt.Println()
	fmt.Println("Status checks not yet implemented.")
	return nil
}
