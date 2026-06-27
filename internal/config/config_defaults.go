package config

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Engine:        defaultEngine(),
		Game:          defaultGame(),
		Container:     defaultContainer(),
		Deploy:        DeployConfig{Target: "gamelift"},
		EC2Fleet:      EC2FleetConfig{ServerSDKVersion: "5.4.0"},
		Anywhere:      defaultAnywhere(),
		GameLift:      defaultGameLift(),
		AWS:           defaultAWS(),
		CI:            defaultCI(),
		DDC:           DDCConfig{Mode: "zen"},
		Observability: defaultObservability(),
		Privacy:       PrivacyConfig{MaskAccountID: true},
	}
}

func defaultEngine() EngineConfig {
	return EngineConfig{
		MaxJobs:         0,
		Backend:         "native",
		DockerImageName: "ludus-engine",
		DockerBaseImage: "ubuntu:22.04",
	}
}

func defaultGame() GameConfig {
	return GameConfig{
		ProjectName: "Lyra",
		Platform:    "linux",
		Arch:        "amd64",
		ServerMap:   "L_Expanse",
	}
}

func defaultContainer() ContainerConfig {
	return ContainerConfig{
		ImageName:  "ludus-server",
		Tag:        "latest",
		ServerPort: 7777,
	}
}

func defaultAnywhere() AnywhereConfig {
	return AnywhereConfig{
		LocationName: "custom-ludus-dev",
		AWSProfile:   "default",
	}
}

func defaultGameLift() GameLiftConfig {
	return GameLiftConfig{
		FleetName:             "ludus-fleet",
		InstanceType:          "c6i.large",
		MaxConcurrentSessions: 1,
		ContainerGroupName:    "ludus-container-group",
	}
}

func defaultAWS() AWSConfig {
	return AWSConfig{
		Region:        "us-east-1",
		ECRRepository: "ludus-server",
		Tags:          map[string]string{"ManagedBy": "ludus"},
	}
}

func defaultCI() CIConfig {
	return CIConfig{
		WorkflowPath: ".github/workflows/ludus-pipeline.yml",
		RunnerDir:    "~/actions-runner",
		RunnerLabels: []string{"self-hosted", "linux", "x64"},
	}
}

func defaultObservability() ObservabilityConfig {
	return ObservabilityConfig{
		Logs: LogsConfig{
			// EnabledPtr nil → Enabled() returns true by default.
			Dir:        ".ludus/logs",
			RetainRuns: 20,
		},
	}
}
