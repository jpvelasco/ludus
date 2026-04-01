package deploy

import (
	"context"
	"fmt"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/gamelift"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/devrecon/ludus/internal/pricing"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/tags"
	"github.com/spf13/cobra"
)

var (
	region       string
	instanceType string
	fleetName    string
	targetFlag   string
	stackName    string
	anywhereIP   string
	ec2Arch      string
	withSession  bool
	destroyAll   bool
)

// Cmd is the top-level deploy command group.
var Cmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the game server to a target",
	Long: `Commands for deploying the game server to a deployment target.

Supported targets: gamelift (default), stack, binary, anywhere, ec2.
Use --target to override the target from ludus.yaml.

Instance type guidance for --instance-type:
  Compute-optimized: c6i.large ($0.085/hr), c6i.xlarge ($0.170/hr) — best for most game servers
  Graviton (ARM64):  c6g.large ($0.068/hr), c7g.large ($0.072/hr) — 20-30% cheaper, requires --arch arm64
  General purpose:   m6i.large ($0.096/hr) — balanced CPU/memory workloads
  Memory-optimized:  r6i.large ($0.126/hr) — open world, many players, large game state`,
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Create a test game session",
	Long:  `Creates a game session on the deployed fleet for testing client connections.`,
	RunE:  runSession,
}

func init() {
	Cmd.PersistentFlags().StringVar(&targetFlag, "target", "", "deployment target: gamelift, stack, binary, anywhere, ec2 (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&region, "region", "", "AWS region (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&instanceType, "instance-type", "", "EC2 instance type (default: from ludus.yaml)")
	Cmd.PersistentFlags().StringVar(&fleetName, "fleet-name", "", "GameLift fleet name (default: from ludus.yaml)")

	Cmd.AddCommand(sessionCmd)
}

// makeDeployer creates a GameLift deployer with flag overrides applied.
// Used by GameLift-specific commands (fleet, session) that need direct Deployer access.
func makeDeployer(cmd *cobra.Command) (*gamelift.Deployer, error) {
	cfg := globals.Cfg

	r := region
	if r == "" {
		r = cfg.AWS.Region
	}
	it := instanceType
	if it == "" {
		it = cfg.GameLift.InstanceType
	}
	fn := fleetName
	if fn == "" {
		fn = cfg.GameLift.FleetName
	}

	// Auto-default instance type based on server architecture
	if resolved, switched := pricing.AutoSwitch(it, cfg.Game.ResolvedArch()); switched {
		fmt.Printf("Note: Switching instance type to %s to match %s server architecture\n", resolved, cfg.Game.ResolvedArch())
		it = resolved
	}

	imageURI := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s:%s",
		cfg.AWS.AccountID, r, cfg.AWS.ECRRepository, cfg.Container.Tag)

	awsCfg, err := awsutil.LoadAWSConfig(cmd.Context(), r)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	return gamelift.NewDeployer(gamelift.DeployOptions{
		Region:             r,
		ImageURI:           imageURI,
		FleetName:          fn,
		InstanceType:       it,
		ContainerGroupName: cfg.GameLift.ContainerGroupName,
		ServerPort:         cfg.Container.ServerPort,
		Tags:               tags.Build(cfg),
	}, awsCfg), nil
}

// resolveTarget resolves a deploy.Target, applying --target flag override and
// flag overrides for GameLift-specific flags (--region, --instance-type, --fleet-name).
func resolveTarget(cmd *cobra.Command) (deploy.Target, error) {
	cfg := globals.Cfg.Clone()

	// Apply flag overrides to config before resolving
	if region != "" {
		cfg.AWS.Region = region
	}
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}

	return globals.ResolveTarget(cmd.Context(), &cfg, targetFlag)
}

// maybeCreateSession creates a game session if --with-session was passed.
func maybeCreateSession(ctx context.Context, sm deploy.SessionManager) error {
	if !withSession {
		return nil
	}
	fmt.Println("\nCreating game session...")
	info, err := sm.CreateSession(ctx, 8)
	if err != nil {
		return fmt.Errorf("session creation failed: %w", err)
	}
	if err := state.UpdateSession(&state.SessionState{
		SessionID: info.SessionID,
		IPAddress: info.IPAddress,
		Port:      info.Port,
	}); err != nil {
		fmt.Printf("Warning: failed to write session state: %v\n", err)
	}
	fmt.Printf("Game session created: %s\n", info.SessionID)
	fmt.Printf("Connect: %s:%d\n", info.IPAddress, info.Port)
	return nil
}

// printPricingHints displays pricing estimate and architecture suggestions.
func printPricingHints(it, arch string) {
	if est := pricing.FormatEstimate(it); est != "" {
		fmt.Println(est)
	}
	if sug := pricing.FormatSuggestion(it, arch); sug != "" {
		fmt.Println(sug)
	}
}

// printNextStep prints the suggested next command based on --with-session.
func printNextStep() {
	if withSession {
		fmt.Println("\nNext: ludus connect")
	} else {
		fmt.Println("\nNext: ludus deploy session")
	}
}

func runSession(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}

	target, err := resolveTarget(cmd)
	if err != nil {
		return err
	}

	sm, ok := target.(deploy.SessionManager)
	if !ok {
		return fmt.Errorf("target %q does not support game sessions", target.Name())
	}

	fmt.Println("Creating game session...")
	info, err := sm.CreateSession(cmd.Context(), 8)
	if err != nil {
		return err
	}

	fmt.Printf("Game session created: %s\n", info.SessionID)
	fmt.Println("\nNext: ludus connect")
	return nil
}
