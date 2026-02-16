package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Load reads configuration from the given YAML file path, merges with defaults,
// and returns a fully populated Config. If path is empty, it searches for
// ludus.yaml in the current directory.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	v := viper.New()
	v.SetConfigType("yaml")

	if path != "" {
		v.SetConfigFile(path)
	} else {
		// Use SetConfigFile with explicit .yaml extension to avoid
		// Viper matching the 'ludus' binary as a config file.
		v.SetConfigFile("ludus.yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return cfg, nil
		}
		// Also handle the case where SetConfigFile was used but file doesn't exist
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Expand relative engine source path to absolute
	if cfg.Engine.SourcePath != "" && !filepath.IsAbs(cfg.Engine.SourcePath) {
		cwd, err := os.Getwd()
		if err == nil {
			cfg.Engine.SourcePath = filepath.Join(cwd, cfg.Engine.SourcePath)
		}
	}

	return cfg, nil
}

// Config holds the full Ludus configuration, typically loaded from ludus.yaml.
type Config struct {
	Engine    EngineConfig    `yaml:"engine"`
	Lyra      LyraConfig      `yaml:"lyra"`
	Container ContainerConfig `yaml:"container"`
	GameLift  GameLiftConfig  `yaml:"gamelift"`
	AWS       AWSConfig       `yaml:"aws"`
}

// EngineConfig holds UE5 engine build settings.
type EngineConfig struct {
	// SourcePath is the path to the Unreal Engine source directory.
	SourcePath string `yaml:"sourcePath"`
	// Version is the engine version tag (e.g. "5.7.3").
	Version string `yaml:"version"`
	// MaxJobs limits parallel compile jobs. 0 = auto-detect based on RAM.
	MaxJobs int `yaml:"maxJobs"`
}

// LyraConfig holds Lyra project build settings.
type LyraConfig struct {
	// ProjectPath is the path to the Lyra .uproject file.
	// If empty, defaults to <engine>/Samples/Games/Lyra/Lyra.uproject.
	ProjectPath string `yaml:"projectPath"`
	// Platform is the target build platform (default: "linux").
	Platform string `yaml:"platform"`
	// ServerMap is the default map for the dedicated server.
	ServerMap string `yaml:"serverMap"`
}

// ContainerConfig holds Docker container settings.
type ContainerConfig struct {
	// ImageName is the Docker image name.
	ImageName string `yaml:"imageName"`
	// Tag is the Docker image tag.
	Tag string `yaml:"tag"`
	// ServerPort is the game server port exposed in the container.
	ServerPort int `yaml:"serverPort"`
}

// GameLiftConfig holds GameLift deployment settings.
type GameLiftConfig struct {
	// FleetName is the name of the GameLift container fleet.
	FleetName string `yaml:"fleetName"`
	// InstanceType is the EC2 instance type for the fleet.
	InstanceType string `yaml:"instanceType"`
	// MaxConcurrentSessions per instance.
	MaxConcurrentSessions int `yaml:"maxConcurrentSessions"`
	// ContainerGroupName is the name of the container group definition.
	ContainerGroupName string `yaml:"containerGroupName"`
}

// AWSConfig holds AWS account and region settings.
type AWSConfig struct {
	// Region is the AWS region for deployment.
	Region string `yaml:"region"`
	// AccountID is the AWS account ID (used for ECR URI construction).
	AccountID string `yaml:"accountId"`
	// ECRRepository is the ECR repository name.
	ECRRepository string `yaml:"ecrRepository"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Engine: EngineConfig{
			MaxJobs: 0,
		},
		Lyra: LyraConfig{
			Platform:  "linux",
			ServerMap: "L_Expanse",
		},
		Container: ContainerConfig{
			ImageName:  "ludus-lyra-server",
			Tag:        "latest",
			ServerPort: 7777,
		},
		GameLift: GameLiftConfig{
			FleetName:             "ludus-lyra-fleet",
			InstanceType:          "c6i.large",
			MaxConcurrentSessions: 1,
			ContainerGroupName:    "ludus-lyra-container-group",
		},
		AWS: AWSConfig{
			Region:        "us-east-1",
			ECRRepository: "ludus-lyra-server",
		},
	}
}
