package config

import "testing"

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	stringChecks := []struct {
		name string
		got  string
		want string
	}{
		{"engine.backend", cfg.Engine.Backend, "native"},
		{"engine.dockerImageName", cfg.Engine.DockerImageName, "ludus-engine"},
		{"engine.dockerBaseImage", cfg.Engine.DockerBaseImage, "ubuntu:22.04"},
		{"game.projectName", cfg.Game.ProjectName, "Lyra"},
		{"game.platform", cfg.Game.Platform, "linux"},
		{"game.arch", cfg.Game.Arch, "amd64"},
		{"game.serverMap", cfg.Game.ServerMap, "L_Expanse"},
		{"container.imageName", cfg.Container.ImageName, "ludus-server"},
		{"container.tag", cfg.Container.Tag, "latest"},
		{"deploy.target", cfg.Deploy.Target, "gamelift"},
		{"gamelift.fleetName", cfg.GameLift.FleetName, "ludus-fleet"},
		{"gamelift.instanceType", cfg.GameLift.InstanceType, "c6i.large"},
		{"gamelift.containerGroupName", cfg.GameLift.ContainerGroupName, "ludus-container-group"},
		{"aws.region", cfg.AWS.Region, "us-east-1"},
		{"aws.ecrRepository", cfg.AWS.ECRRepository, "ludus-server"},
		{"anywhere.locationName", cfg.Anywhere.LocationName, "custom-ludus-dev"},
		{"anywhere.awsProfile", cfg.Anywhere.AWSProfile, "default"},
		{"ec2fleet.serverSdkVersion", cfg.EC2Fleet.ServerSDKVersion, "5.4.0"},
		{"ci.workflowPath", cfg.CI.WorkflowPath, ".github/workflows/ludus-pipeline.yml"},
		{"ci.runnerDir", cfg.CI.RunnerDir, "~/actions-runner"},
		{"ddc.mode", cfg.DDC.Mode, "local"},
		{"ddc.localPath", cfg.DDC.LocalPath, ""},
	}
	for _, tt := range stringChecks {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}

	t.Run("engine.maxJobs", func(t *testing.T) {
		if cfg.Engine.MaxJobs != 0 {
			t.Errorf("got %d, want 0", cfg.Engine.MaxJobs)
		}
	})
	t.Run("container.serverPort", func(t *testing.T) {
		if cfg.Container.ServerPort != 7777 {
			t.Errorf("got %d, want 7777", cfg.Container.ServerPort)
		}
	})
	t.Run("gamelift.maxConcurrentSessions", func(t *testing.T) {
		if cfg.GameLift.MaxConcurrentSessions != 1 {
			t.Errorf("got %d, want 1", cfg.GameLift.MaxConcurrentSessions)
		}
	})
	t.Run("aws.tags ManagedBy", func(t *testing.T) {
		if cfg.AWS.Tags["ManagedBy"] != "ludus" {
			t.Errorf("got %q, want %q", cfg.AWS.Tags["ManagedBy"], "ludus")
		}
	})
	t.Run("ci.runnerLabels count", func(t *testing.T) {
		if len(cfg.CI.RunnerLabels) != 3 {
			t.Errorf("got %d labels, want 3", len(cfg.CI.RunnerLabels))
		}
	})
}
