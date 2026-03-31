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
	region := resolveRegion(cfg.AWS.Region)

	awsCfg, err := awsutil.LoadAWSConfig(cmd.Context(), region)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	ecrRepoNames := resolveECRRepos(cfg.AWS.ECRRepository, cfg.Engine.DockerImageName)
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

	printInventory(inv, region)
	return nil
}

// resolveRegion returns the CLI flag value or falls back to the config value.
func resolveRegion(cfgRegion string) string {
	if regionFlag != "" {
		return regionFlag
	}
	return cfgRegion
}

// resolveECRRepos builds the deduplicated list of ECR repository names to scan.
func resolveECRRepos(serverRepo, engineRepo string) []string {
	if serverRepo == "" {
		serverRepo = "ludus-server"
	}
	if engineRepo == "" {
		engineRepo = "ludus-engine"
	}
	repos := []string{serverRepo}
	if engineRepo != serverRepo {
		repos = append(repos, engineRepo)
	}
	return repos
}

// printInventory formats and prints the scanned inventory to stdout.
func printInventory(inv *inventory.Inventory, region string) {
	if len(inv.Resources) == 0 {
		fmt.Printf("No ludus-managed resources found in %s.\n", region)
		return
	}

	fmt.Printf("Ludus Resources (%s)\n\n", region)
	fmt.Printf("  %-30s  %-40s  %s\n", "TYPE", "NAME", "DETAIL")
	for _, r := range inv.Resources {
		fmt.Printf("  %-30s  %-40s  %s\n", r.Type, r.Name, resourceDetail(r))
	}
	fmt.Println()
}

// resourceDetail returns the best available detail string for a resource row.
func resourceDetail(r inventory.Resource) string {
	if r.Detail != "" {
		return r.Detail
	}
	if r.Status != "" {
		return r.Status
	}
	return "--"
}
