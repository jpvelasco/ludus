package deploy

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/cleanup"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Tear down all deployed resources",
	Long: `Destroys all resources created by Ludus for the active deployment target.

For GameLift: deletes fleet, container group definition, and IAM role.
For stack: deletes the CloudFormation stack (all resources removed atomically).
For binary: removes the output directory.
For anywhere: stops server, deregisters compute, deletes fleet and location.
For ec2: deletes fleet, build, S3 object, and IAM role.

Resources that don't exist are skipped gracefully.

Use --all to destroy resources across all target types.`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVar(&destroyAll, "all", false, "destroy resources across all target types")
	Cmd.AddCommand(destroyCmd)
}

func runDestroy(cmd *cobra.Command, args []string) error {
	if destroyAll {
		return runDestroyAll(cmd)
	}

	target, err := resolveTarget(cmd)
	if err != nil {
		return err
	}

	fmt.Printf("Destroying %s resources...\n", target.Name())
	if err := target.Destroy(cmd.Context()); err != nil {
		return err
	}

	fmt.Printf("\nAll %s resources destroyed.\n", target.Name())
	return nil
}

func runDestroyAll(cmd *cobra.Command) error {
	cfg := globals.Cfg

	// Apply flag overrides
	if region != "" {
		cfg.AWS.Region = region
	}

	targets := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	destroyed := 0

	for _, name := range targets {
		target, err := globals.ResolveTarget(cmd.Context(), cfg, name)
		if err != nil {
			if globals.Verbose {
				fmt.Printf("  Skipping %s: %v\n", name, err)
			}
			continue
		}

		fmt.Printf("Destroying %s resources...\n", name)
		if err := target.Destroy(cmd.Context()); err != nil {
			fmt.Printf("  %s: %v (continuing)\n", name, err)
			continue
		}
		destroyed++
		fmt.Printf("  %s: destroyed\n", name)
	}

	if destroyed == 0 {
		fmt.Println("\nNo resources found to destroy.")
	} else {
		fmt.Printf("\nDestroyed resources across %d target(s).\n", destroyed)
	}

	// Destroy shared resources (ECR repos, S3 bucket)
	fmt.Println("\nDestroying shared resources...")
	awsCfg, err := awsutil.LoadAWSConfig(cmd.Context(), cfg.AWS.Region)
	if err != nil {
		fmt.Printf("  Warning: could not load AWS config: %v\n", err)
		return nil
	}

	cleaner := cleanup.NewCleaner(awsCfg)

	// Delete game server ECR repository
	ecrRepo := cfg.AWS.ECRRepository
	if ecrRepo == "" {
		ecrRepo = "ludus-server"
	}
	if err := cleaner.DeleteECRRepository(cmd.Context(), ecrRepo); err != nil {
		fmt.Printf("  ECR %s: %v (continuing)\n", ecrRepo, err)
	}

	// Delete engine ECR repository (if different)
	engineRepo := cfg.Engine.DockerImageName
	if engineRepo == "" {
		engineRepo = "ludus-engine"
	}
	if engineRepo != ecrRepo {
		if err := cleaner.DeleteECRRepository(cmd.Context(), engineRepo); err != nil {
			fmt.Printf("  ECR %s: %v (continuing)\n", engineRepo, err)
		}
	}

	// Delete S3 builds bucket
	accountID := cfg.AWS.AccountID
	if accountID == "" {
		stsClient := sts.NewFromConfig(awsCfg)
		identity, stsErr := stsClient.GetCallerIdentity(cmd.Context(), &sts.GetCallerIdentityInput{})
		if stsErr == nil {
			accountID = aws.ToString(identity.Account)
		}
	}
	if accountID != "" {
		bucket := fmt.Sprintf("ludus-builds-%s", accountID)
		if err := cleaner.DeleteS3Bucket(cmd.Context(), bucket); err != nil {
			fmt.Printf("  S3 %s: %v (continuing)\n", bucket, err)
		}
	}

	return nil
}
