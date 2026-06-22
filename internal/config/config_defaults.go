package config

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Engine: EngineConfig{
			MaxJobs:         0,
			Backend:         "native",
			DockerImageName: "ludus-engine",
			DockerBaseImage: "ubuntu:22.04",
		},
		Game: GameConfig{
			ProjectName: "Lyra",
			Platform:    "linux",
			Arch:        "amd64",
			ServerMap:   "L_Expanse",
		},
		Container: ContainerConfig{
			ImageName:  "ludus-server",
			Tag:        "latest",
			ServerPort: 7777,
		},
		Deploy: DeployConfig{
			Target: "gamelift",
		},
		EC2Fleet: EC2FleetConfig{
			ServerSDKVersion: "5.4.0",
		},
		Anywhere: AnywhereConfig{
			LocationName: "custom-ludus-dev",
			AWSProfile:   "default",
		},
		GameLift: GameLiftConfig{
			FleetName:             "ludus-fleet",
			InstanceType:          "c6i.large",
			MaxConcurrentSessions: 1,
			ContainerGroupName:    "ludus-container-group",
		},
		AWS: AWSConfig{
			Region:        "us-east-1",
			ECRRepository: "ludus-server",
			Tags:          map[string]string{"ManagedBy": "ludus"},
		},
		CI: CIConfig{
			WorkflowPath: ".github/workflows/ludus-pipeline.yml",
			RunnerDir:    "~/actions-runner",
			RunnerLabels: []string{"self-hosted", "linux", "x64"},
		},
		DDC: DDCConfig{
			Mode: "local",
		},
		Observability: ObservabilityConfig{
			Logs: LogsConfig{
				// EnabledPtr nil → Enabled() returns true by default.
				Dir:        ".ludus/logs",
				RetainRuns: 20,
			},
		},
		Privacy: PrivacyConfig{
			MaskAccountID: true,
		},
	}
}
