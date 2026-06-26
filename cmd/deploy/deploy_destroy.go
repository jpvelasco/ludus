package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/cleanup"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Tear down deployed resources for the active target",
	Long: `Destroys the resources created by Ludus for the active deployment target.

By default this is scoped and safe — it removes only the EPHEMERAL deployment
resources and leaves your durable build artifacts (ECR repositories, S3 build
buckets) intact:

  GameLift: fleet, container group definition, IAM role
  stack:    the CloudFormation stack (all resources removed atomically)
  ec2:      fleet, GameLift build, the uploaded S3 build object, IAM role
  anywhere: stops the server, deregisters compute, deletes fleet and location
  binary:   the output directory

Resources that don't exist are skipped gracefully.

Flags (two independent axes):
  --all-targets   widen the teardown to every target type, not just the active one
  --purge         ALSO delete durable artifacts (ECR repositories + S3 build
                  buckets). Prompts for confirmation unless --yes is given.
  --yes / -y      skip the --purge confirmation prompt (for CI/automation)

Examples:
  ludus deploy destroy                       # this target's fleet/group/IAM only
  ludus deploy destroy --all-targets         # sweep every target, artifacts kept
  ludus deploy destroy --purge               # this target + ECR/S3 (prompts)
  ludus deploy destroy --all-targets --purge --yes  # full wipe, no prompt`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVar(&destroyAllTgts, "all-targets", false, "tear down every deploy target, not just the active one")
	destroyCmd.Flags().BoolVar(&destroyPurge, "purge", false, "also delete durable artifacts (ECR repositories + S3 build buckets)")
	destroyCmd.Flags().BoolVarP(&destroyYes, "yes", "y", false, "skip the --purge confirmation prompt")
	Cmd.AddCommand(destroyCmd)
}

// destroyScope is the resolved teardown plan: how wide (sweep) and how deep
// (durable). It is pure data so runDestroy can stay a thin dispatcher.
type destroyScope struct {
	sweep   bool // tear down all targets, not just the active one
	durable bool // also delete durable artifacts (ECR repos, S3 build buckets)
}

func resolveDestroyScope(allTargets, purge bool) destroyScope {
	return destroyScope{sweep: allTargets, durable: purge}
}

func runDestroy(cmd *cobra.Command, args []string) error {
	scope := resolveDestroyScope(destroyAllTgts, destroyPurge)
	cfg := globals.Cfg.Clone()
	if region != "" {
		cfg.AWS.Region = region
	}

	if scope.durable && !confirmPurge(cmd.OutOrStdout(), cmd.InOrStdin(), purgeItems(&cfg), destroyYes) {
		fmt.Fprintln(cmd.OutOrStdout(), "Aborted; no resources were deleted.")
		return nil
	}

	if scope.sweep {
		destroyAllTargets(cmd.Context(), &cfg)
	} else if err := destroyActiveTarget(cmd); err != nil {
		return err
	}

	if scope.durable {
		return cleanupSharedResources(cmd.Context(), &cfg)
	}
	return nil
}

// destroyActiveTarget tears down only the ephemeral resources of the configured
// target. Durable artifacts are never touched here (see --purge).
func destroyActiveTarget(cmd *cobra.Command) error {
	target, err := resolveTarget(cmd)
	if err != nil {
		return err
	}
	fmt.Printf("Destroying %s resources...\n", target.Name())
	if err := target.Destroy(cmd.Context()); err != nil {
		return err
	}
	fmt.Printf("\n%s resources destroyed.\n", target.Name())
	return nil
}

// purgeItems lists the durable artifacts that --purge would delete, for the
// confirmation prompt.
func purgeItems(cfg *config.Config) []string {
	ecrRepo := cfg.AWS.ECRRepository
	if ecrRepo == "" {
		ecrRepo = "ludus-server"
	}
	return []string{
		fmt.Sprintf("ECR repository: %s (and all images)", ecrRepo),
		fmt.Sprintf("S3 build bucket: ludus-builds-%s", accountIDLabel(cfg.AWS.AccountID)),
	}
}

func accountIDLabel(id string) string {
	if id == "" {
		return "<account-id>"
	}
	return id
}

// confirmPurge prints the durable items that will be deleted and reads a y/N
// answer. Returns true when the user confirms or skip (--yes) is set.
func confirmPurge(w io.Writer, in io.Reader, items []string, skip bool) bool {
	fmt.Fprintln(w, "--purge will permanently delete these durable artifacts:")
	for _, it := range items {
		fmt.Fprintf(w, "  - %s\n", it)
	}
	if skip {
		return true
	}
	fmt.Fprint(w, "Continue? [y/N]: ")
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func destroyAllTargets(ctx context.Context, cfg *config.Config) {
	targets := []string{"gamelift", "stack", "ec2", "anywhere", "binary"}
	destroyed := 0

	for _, name := range targets {
		target, err := globals.ResolveTarget(ctx, cfg, name)
		if err != nil {
			if globals.Verbose {
				fmt.Printf("  Skipping %s: %v\n", name, err)
			}
			continue
		}

		fmt.Printf("Destroying %s resources...\n", name)
		if err := target.Destroy(ctx); err != nil {
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
}

func cleanupSharedResources(ctx context.Context, cfg *config.Config) error {
	fmt.Println("\nDestroying shared resources...")
	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		fmt.Printf("  Warning: could not load AWS config: %v\n", err)
		return nil
	}

	cleaner := cleanup.NewCleaner(awsCfg)
	cleanupECRRepos(ctx, cleaner, cfg)
	cleanupS3Bucket(ctx, cleaner, awsCfg, cfg)
	return nil
}

func cleanupECRRepos(ctx context.Context, cleaner *cleanup.Cleaner, cfg *config.Config) {
	ecrRepo := cfg.AWS.ECRRepository
	if ecrRepo == "" {
		ecrRepo = "ludus-server"
	}
	if err := cleaner.DeleteECRRepository(ctx, ecrRepo); err != nil {
		fmt.Printf("  ECR %s: %v (continuing)\n", ecrRepo, err)
	}
}

func cleanupS3Bucket(ctx context.Context, cleaner *cleanup.Cleaner, awsCfg aws.Config, cfg *config.Config) {
	accountID := resolveAccountID(ctx, awsCfg, cfg.AWS.AccountID)
	if accountID == "" {
		return
	}
	bucket := fmt.Sprintf("ludus-builds-%s", accountID)
	if err := cleaner.DeleteS3Bucket(ctx, bucket); err != nil {
		fmt.Printf("  S3 %s: %v (continuing)\n", bucket, err)
	}
}

func resolveAccountID(ctx context.Context, awsCfg aws.Config, configured string) string {
	if configured != "" {
		return configured
	}
	stsClient := sts.NewFromConfig(awsCfg)
	id, _ := awsenv.AccountID(ctx, stsClient)
	return id
}
