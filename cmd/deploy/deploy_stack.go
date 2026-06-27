package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/awsenv"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/diagnose"
	"github.com/jpvelasco/ludus/internal/prereq"
	"github.com/jpvelasco/ludus/internal/pricing"
	"github.com/jpvelasco/ludus/internal/stack"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/tags"
	"github.com/spf13/cobra"
)

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Deploy via CloudFormation stack",
	Long: `Deploys a CloudFormation stack that atomically provisions:

  - IAM role for GameLift container fleet
  - Container group definition
  - Container fleet with inbound permissions

The stack provides atomic deployments with automatic rollback on failure.
Use --stack-name to override the default stack name (ludus-<fleet-name>).`,
	RunE: runStack,
}

func init() {
	stackCmd.Flags().StringVar(&stackName, "stack-name", "", "CloudFormation stack name (default: ludus-<fleet-name>)")
	stackCmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	Cmd.AddCommand(stackCmd)
}

func applyStackFlags(ctx context.Context, cfg *config.Config) (env awsenv.Env, imageURI, sn, fn string, err error) {
	if region != "" {
		cfg.AWS.Region = region
	}
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}

	fn = fleetName
	if fn == "" {
		fn = cfg.GameLift.FleetName
	}

	if resolved, switched := pricing.AutoSwitch(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type from %s to %s to match %s server architecture\n",
			cfg.GameLift.InstanceType, resolved, cfg.Game.ResolvedArch())
		cfg.GameLift.InstanceType = resolved
	}

	sn = stackName
	if sn == "" {
		sn = fmt.Sprintf("ludus-%s", fn)
	}

	env, err = awsenv.NewResolver(globals.DryRun).Resolve(ctx, cfg, awsenv.Requirements{Account: true, Region: true})
	if err != nil {
		return awsenv.Env{}, "", "", "", err
	}
	imageURI, err = awsenv.ImageURI(env, cfg.AWS.ECRRepository, cfg.Container.Tag)
	if err != nil {
		return awsenv.Env{}, "", "", "", err
	}
	return env, imageURI, sn, fn, nil
}

func saveStackState(result *stack.StackResult) {
	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   result.FleetID,
		StackName: result.StackName,
		Status:    result.Status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "stack",
		Status:     result.Status,
		Detail:     fmt.Sprintf("stack %s, fleet %s", result.StackName, result.FleetID),
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write deploy state: %v\n", err)
	}
}

func runStack(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg.Clone()
	env, imageURI, sn, fn, err := applyStackFlags(cmd.Context(), &cfg)
	if err != nil {
		return err
	}

	checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}
	printPricingHints(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch())

	if globals.DryRun {
		fmt.Println("Dry run — would deploy CloudFormation stack (no AWS calls made).")
		return nil
	}

	start := time.Now()
	deployer := stack.NewStackDeployer(stack.StackOptions{
		StackName:          sn,
		Region:             env.Region,
		ImageURI:           imageURI,
		FleetName:          fn,
		InstanceType:       cfg.GameLift.InstanceType,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		ServerSDKVersion:   "5.4.0",
		Tags:               tags.Build(&cfg),
	}, env.AWSConfig)

	result, err := deployer.Deploy(cmd.Context())
	if err != nil {
		return diagnose.DeployError(err, "stack")
	}

	saveStackState(result)

	elapsed := time.Since(start)
	fmt.Printf("\nStack deployed: %s (status: %s)\n", result.StackName, result.Status)
	if result.FleetID != "" {
		fmt.Printf("Fleet ID: %s\n", result.FleetID)
	}
	fmt.Printf("Duration: %s\n", elapsed.Round(time.Second))
	if err := maybeCreateSession(cmd.Context(), stack.NewTargetAdapter(deployer)); err != nil {
		return err
	}
	printNextStep()
	return nil
}
