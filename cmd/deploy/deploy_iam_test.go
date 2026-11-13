package deploy

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/gamelift"
)

func TestFleetRejectsIAMPathBeforePrerequisites(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "dry run"}[dryRun], func(t *testing.T) {
			cfg := config.Defaults()
			cfg.GameLift.IAM.Path = "/project/"
			globals.SetGlobals(t, cfg)
			globals.DryRun = dryRun
			if err := runFleet(newCommand(), nil); err == nil || !strings.Contains(err.Error(), "root-path role") {
				t.Fatalf("runFleet error = %v", err)
			}
		})
	}
}

func TestFleetPlanIAMDetails(t *testing.T) {
	for _, tt := range []struct {
		name, path, boundary, want string
	}{
		{"defaults", "", "", "path /, no boundary"},
		{"boundary", "/", "arn:aws:iam::123456789012:policy/example", "path /, boundary arn:aws:iam::123456789012:policy/example"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := gamelift.NewDeployer(gamelift.DeployOptions{
				IAMRoleName: "example-role", IAMPath: tt.path, IAMPermissionsBoundary: tt.boundary,
			}, aws.Config{})
			out := captureStdout(func() { printFleetPlan(d.Plan()) })
			if !strings.Contains(out, "example-role ("+tt.want+")") {
				t.Fatalf("missing IAM plan details: %s", out)
			}
		})
	}
}
