package resources

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/inventory"
	"github.com/spf13/cobra"
)

var regionFlag string

// Cmd is the resources command.
var Cmd = &cobra.Command{
	Use:   "resources",
	Short: "List ludus-managed AWS resources",
	Long: `Scans the configured AWS region for resources created by Ludus.

Discovers resources by ManagedBy=ludus tag and known naming patterns
(ECR repositories, S3 build buckets).`,
	RunE: runResources,
}

func init() {
	Cmd.Flags().StringVar(&regionFlag, "region", "", "AWS region (default: from config or AWS_DEFAULT_REGION)")
}

func runResources(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	region := regionFlag
	if region == "" {
		region = cfg.AWS.Region
	}

	awsCfg, err := awsutil.LoadAWSConfig(cmd.Context(), region)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	// Known ECR repo names
	ecrRepo := cfg.AWS.ECRRepository
	if ecrRepo == "" {
		ecrRepo = "ludus-server"
	}
	engineRepo := cfg.Engine.DockerImageName
	if engineRepo == "" {
		engineRepo = "ludus-engine"
	}
	ecrRepoNames := []string{ecrRepo}
	if engineRepo != ecrRepo {
		ecrRepoNames = append(ecrRepoNames, engineRepo)
	}

	scanner := inventory.NewScanner(awsCfg, region, ecrRepoNames, "ludus-builds-")
	inv, err := scanner.Scan(cmd.Context())
	if err != nil {
		return fmt.Errorf("scanning resources: %w", err)
	}

	if globals.JSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(inv)
	}

	if len(inv.Resources) == 0 {
		fmt.Printf("No ludus-managed resources found in %s.\n", region)
		return nil
	}

	fmt.Printf("Ludus Resources (%s)\n\n", region)
	fmt.Printf("  %-30s  %-40s  %s\n", "TYPE", "NAME", "DETAIL")
	for _, r := range inv.Resources {
		detail := r.Detail
		if detail == "" && r.Status != "" {
			detail = r.Status
		}
		if detail == "" {
			detail = "--"
		}
		fmt.Printf("  %-30s  %-40s  %s\n", r.Type, r.Name, detail)
	}
	fmt.Println()
	return nil
}
