package deploy

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/diagnose"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/stack"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/tags"
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

func applyStackFlags(cfg *config.Config) (imageURI, sn, fn string) {
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

	r := cfg.AWS.Region
	imageURI = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, r, cfg.AWS.ECRRepository, cfg.Container.Tag)
	return imageURI, sn, fn
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
	cfg := *globals.Cfg
	imageURI, sn, fn := applyStackFlags(&cfg)

	checker := prereq.NewChecker(cfg.Engine.SourcePath, cfg.Engine.Version, false, &cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}
	printPricingHints(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch())

	awsCfg, err := awsutil.LoadAWSConfig(cmd.Context(), cfg.AWS.Region)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	start := time.Now()
	deployer := stack.NewStackDeployer(stack.StackOptions{
		StackName:          sn,
		Region:             cfg.AWS.Region,
		ImageURI:           imageURI,
		FleetName:          fn,
		InstanceType:       cfg.GameLift.InstanceType,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		ServerSDKVersion:   "5.4.0",
		Tags:               tags.Build(&cfg),
	}, awsCfg)

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
