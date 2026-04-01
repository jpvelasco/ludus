package deploy

import (
	"fmt"
	"time"

	"github.com/devrecon/ludus/cmd/globals"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/diagnose"
	"github.com/devrecon/ludus/internal/prereq"
	"github.com/spf13/cobra"
)

var ec2Cmd = &cobra.Command{
	Use:   "ec2",
	Short: "Deploy a GameLift Managed EC2 fleet",
	Long: `Deploys the server build to a GameLift Managed EC2 fleet by:

  1. Zipping the server build with the Game Server Wrapper
  2. Uploading to S3
  3. Creating a GameLift Build
  4. Creating an EC2 fleet with runtime configuration
  5. Waiting for fleet to become ACTIVE

No Docker or containers required — GameLift runs the server binary directly on EC2.`,
	RunE: runEC2,
}

func init() {
	ec2Cmd.Flags().StringVar(&ec2Arch, "arch", "", `target CPU architecture: amd64, arm64 (default: from ludus.yaml)`)
	ec2Cmd.Flags().BoolVar(&withSession, "with-session", false, "create a game session after deployment")
	Cmd.AddCommand(ec2Cmd)
}

func applyEC2Flags(cfg *config.Config) {
	if region != "" {
		cfg.AWS.Region = region
	}
	if instanceType != "" {
		cfg.GameLift.InstanceType = instanceType
	}
	if fleetName != "" {
		cfg.GameLift.FleetName = fleetName
	}
	if ec2Arch != "" {
		cfg.Game.Arch = ec2Arch
	}
}

func runEC2(cmd *cobra.Command, args []string) error {
	checker := prereq.NewChecker(globals.Cfg.Engine.SourcePath, globals.Cfg.Engine.Version, false, &globals.Cfg.Game)
	if err := prereq.Validate(checker.CheckAWSReady()); err != nil {
		return err
	}

	cfg := *globals.Cfg
	applyEC2Flags(&cfg)

	target, err := globals.ResolveTarget(cmd.Context(), &cfg, "ec2")
	if err != nil {
		return err
	}

	printPricingHints(cfg.GameLift.InstanceType, cfg.Game.ResolvedArch())

	serverBuildDir := config.ResolveServerBuildDir(&cfg)
	if serverBuildDir == "" {
		return fmt.Errorf("could not determine server build directory; set game.projectPath in ludus.yaml")
	}

	start := time.Now()
	result, err := target.Deploy(cmd.Context(), deploy.DeployInput{
		ServerBuildDir: serverBuildDir,
		ServerPort:     cfg.Container.ServerPort,
	})
	if err != nil {
		return diagnose.DeployError(err, "ec2")
	}

	elapsed := time.Since(start)
	fmt.Printf("\nEC2 fleet deployed: %s\n", result.Detail)
	fmt.Printf("Duration: %s\n", elapsed.Round(time.Second))
	if sm, ok := target.(deploy.SessionManager); ok {
		if err := maybeCreateSession(cmd.Context(), sm); err != nil {
			return err
		}
	}
	printNextStep()
	return nil
}
