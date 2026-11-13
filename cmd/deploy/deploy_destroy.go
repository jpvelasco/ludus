package deploy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/cleanup"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Tear down deployed resources for the active target",
	Long: `Destroys the resources created by Ludus for the active deployment target.

By default it removes the EPHEMERAL deployment
resources and leaves your durable build artifacts (ECR repositories, S3 build
buckets) intact:

  GameLift: fleet, container group definition, IAM role
  stack:    the CloudFormation stack (template and service deletion rules apply)
  ec2:      fleet, GameLift build, the uploaded S3 build object, IAM role
  anywhere: stops the server, deregisters compute, deletes fleet and location
  binary:   the output directory

Resources that don't exist are skipped gracefully.

--dry-run lists local candidates and unresolved live conditions without AWS calls
or changes. --json returns the plan as one JSON document. Names come from the
current configuration and active profile; older group/role names cannot be
reconstructed after configuration changes.

Durable artifacts (ECR repositories + S3 build buckets) are NOT deleted by a
plain destroy. They are expensive to recreate, so they are kept by default —
but that means overnight cost and orphaned storage if you forget to clean up.
Run with --purge to delete the named server repository and build bucket.

Flags (two independent axes):
  --all-targets   widen the teardown to every target type, not just the active one
  --purge         ALSO delete durable artifacts (ECR repositories + S3 build
                  buckets). Prompts for confirmation unless --yes is given.
  --yes / -y      skip the --purge confirmation prompt (for CI/automation)

Examples:
  ludus deploy destroy                       # this target's fleet/group/IAM only
  ludus deploy destroy --all-targets         # sweep every target, artifacts kept
  ludus deploy destroy --purge               # this target + ECR/S3 (prompts)
  ludus deploy destroy --all-targets --purge --yes  # this profile and named artifacts, no prompt`,
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
	applyDeployFlags(&cfg)

	if globals.DryRun {
		return printDestroyPreview(&cfg, scope)
	}

	if scope.durable && !confirmPurge(cmd.OutOrStdout(), cmd.InOrStdin(), purgeItems(&cfg), destroyYes) {
		fmt.Fprintln(cmd.OutOrStdout(), "Aborted; no resources were deleted.")
		return nil
	}

	var sweepErr error
	if scope.sweep {
		sweepErr = destroyAllTargets(cmd.Context(), &cfg)
	} else if err := destroyActiveTarget(cmd, &cfg); err != nil {
		return err
	}

	if scope.durable {
		return errors.Join(sweepErr, cleanupSharedResources(cmd.Context(), &cfg))
	}
	return sweepErr
}

func printDestroyPreview(cfg *config.Config, scope destroyScope) error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading destroy preview state: %w", err)
	}
	plan, planErr := globals.PlanDestroy(cfg, st, state.ActiveProfile(), targetFlag, scope.sweep, scope.durable)
	if plan == nil {
		return planErr
	}
	if globals.JSONOutput {
		return errors.Join(planErr, json.NewEncoder(os.Stdout).Encode(plan))
	}
	fmt.Print(deploy.RenderDestroyPlan(plan))
	return planErr
}

// destroyActiveTarget tears down only the ephemeral resources of the configured
// target. Durable artifacts are never touched here (see --purge).
func destroyActiveTarget(cmd *cobra.Command, cfg *config.Config) error {
	target, err := globals.ResolveTarget(cmd.Context(), cfg, targetFlag)
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
	names := cleanup.ResolveDurableNames(cfg, cfg.AWS.AccountID)
	bucket := names.S3Bucket
	if bucket == "" {
		bucket = "resolved during execution (account-derived default)"
	}
	return []string{
		fmt.Sprintf("ECR repository: %s (and all images)", names.ECRRepository),
		fmt.Sprintf("S3 build bucket: %s", bucket),
	}
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

// destroyAllTargets sweeps every deploy target, printing and collecting each
// failure so the sweep continues; it returns an aggregate error when any
// target failed to tear down.
func destroyAllTargets(ctx context.Context, cfg *config.Config) error {
	destroyed := 0
	var failures []string

	for _, resolved := range globals.ResolveDestroyTargets(ctx, cfg) {
		name := resolved.Name
		ok, err := destroySweepTarget(ctx, resolved)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		}
		if ok {
			destroyed++
		}
	}

	printDestroySummary(destroyed, len(failures))
	if len(failures) > 0 {
		return fmt.Errorf("destroy sweep failed for %d target(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

// destroySweepTarget tears down one named target. A resolution failure means
// "nothing to destroy for this target" and returns false with no error; a
// Destroy failure is printed and returned so the sweep can continue.
func destroySweepTarget(ctx context.Context, resolved globals.DestroyTarget) (bool, error) {
	name, target, err := resolved.Name, resolved.Target, resolved.Error
	if err != nil {
		if globals.Verbose {
			fmt.Printf("  Skipping %s: %v\n", name, err)
		}
		return false, nil
	}

	fmt.Printf("Destroying %s resources...\n", name)
	if err := target.Destroy(ctx); err != nil {
		fmt.Printf("  %s: %v (continuing)\n", name, err)
		return false, err
	}
	fmt.Printf("  %s: destroyed\n", name)
	return true, nil
}

// printDestroySummary reports how the sweep went. A run with failures gets an
// explicit summary instead of the "no resources" message.
func printDestroySummary(destroyed, failed int) {
	switch {
	case failed > 0:
		fmt.Printf("\nDestroyed resources across %d target(s); %d failed.\n", destroyed, failed)
	case destroyed == 0:
		fmt.Println("\nNo resources found to destroy.")
	default:
		fmt.Printf("\nDestroyed resources across %d target(s).\n", destroyed)
	}
}

// cleanupSharedResources deletes the durable build artifacts (ECR repository,
// S3 build bucket). Configuration or deletion failures surface as errors so
// the CLI exit status reflects a failed purge.
func cleanupSharedResources(ctx context.Context, cfg *config.Config) error {
	fmt.Println("\nDestroying shared resources...")
	awsCfg, err := awsutil.LoadAWSConfig(ctx, cfg.AWS.Region)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	return cleanupDurableArtifacts(ctx, cleanup.NewCleaner(awsCfg), awsCfg, cfg)
}

// cleanupDurableArtifacts deletes the durable artifacts through cl, printing
// each failure and returning an aggregate when any deletion fails.
func cleanupDurableArtifacts(ctx context.Context, cl cleanup.Deleter, awsCfg aws.Config, cfg *config.Config) error {
	var failures []string
	if err := cleanupECRRepos(ctx, cl, cfg); err != nil {
		failures = append(failures, err.Error())
	}
	if err := cleanupS3Bucket(ctx, cl, awsCfg, cfg); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return fmt.Errorf("purge failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func cleanupECRRepos(ctx context.Context, cl cleanup.Deleter, cfg *config.Config) error {
	repo := cleanup.ResolveDurableNames(cfg, cfg.AWS.AccountID).ECRRepository
	if err := cl.DeleteECRRepository(ctx, repo); err != nil {
		fmt.Printf("  ECR %s: %v (continuing)\n", repo, err)
		return fmt.Errorf("ECR repository %s: %w", repo, err)
	}
	return nil
}

func cleanupS3Bucket(ctx context.Context, cl cleanup.Deleter, awsCfg aws.Config, cfg *config.Config) error {
	account := cfg.AWS.AccountID
	if cfg.EC2Fleet.S3Bucket == "" {
		account = resolveAccountID(ctx, awsCfg, account)
	}
	bucket := cleanup.ResolveDurableNames(cfg, account).S3Bucket
	if bucket == "" {
		return nil
	}
	if err := cl.DeleteS3Bucket(ctx, bucket); err != nil {
		fmt.Printf("  S3 %s: %v (continuing)\n", bucket, err)
		return fmt.Errorf("S3 bucket %s: %w", bucket, err)
	}
	return nil
}

func resolveAccountID(ctx context.Context, awsCfg aws.Config, configured string) string {
	if configured != "" {
		return configured
	}
	stsClient := sts.NewFromConfig(awsCfg)
	id, err := awsenv.AccountID(ctx, stsClient)
	if err != nil {
		fmt.Printf("  Warning: could not resolve account ID: %v\n", err)
	}
	return id
}
