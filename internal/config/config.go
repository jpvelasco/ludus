package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
		v.SetConfigFile("ludus.yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		return handleReadError(cfg, err)
	}

	migrateLyraKey(v)

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	resolveEnginePath(cfg)
	return cfg, nil
}

// handleReadError returns defaults for missing files, or wraps real errors.
func handleReadError(cfg *Config, err error) (*Config, error) {
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return cfg, nil
	}
	if os.IsNotExist(err) {
		return cfg, nil
	}
	return nil, fmt.Errorf("reading config: %w", err)
}

// migrateLyraKey copies deprecated 'lyra' keys into 'game' namespace.
func migrateLyraKey(v *viper.Viper) {
	if !v.IsSet("lyra") || v.IsSet("game") {
		return
	}
	fmt.Fprintln(os.Stderr, "WARNING: 'lyra:' config key is deprecated, rename to 'game:' in ludus.yaml")
	for _, key := range []string{
		"projectPath", "projectName", "contentSourcePath", "serverTarget",
		"clientTarget", "gameTarget", "platform", "skipCook", "serverMap",
		"contentValidation",
	} {
		if v.IsSet("lyra." + key) {
			v.Set("game."+key, v.Get("lyra."+key))
		}
	}
}

// resolveEnginePath expands a relative engine source path to absolute.
func resolveEnginePath(cfg *Config) {
	if cfg.Engine.SourcePath == "" || filepath.IsAbs(cfg.Engine.SourcePath) {
		return
	}
	if cwd, err := os.Getwd(); err == nil {
		cfg.Engine.SourcePath = filepath.Join(cwd, cfg.Engine.SourcePath)
	}
}

// Config holds the full Ludus configuration, typically loaded from ludus.yaml.
type Config struct {
	Engine    EngineConfig    `yaml:"engine"`
	Game      GameConfig      `yaml:"game"`
	Container ContainerConfig `yaml:"container"`
	Deploy    DeployConfig    `yaml:"deploy"`
	GameLift  GameLiftConfig  `yaml:"gamelift"`
	EC2Fleet  EC2FleetConfig  `yaml:"ec2fleet"`
	Anywhere  AnywhereConfig  `yaml:"anywhere"`
	AWS       AWSConfig       `yaml:"aws"`
	CI        CIConfig        `yaml:"ci"`
	DDC       DDCConfig       `yaml:"ddc"`
}

// EngineConfig holds UE5 engine build settings.
type EngineConfig struct {
	// SourcePath is the path to the Unreal Engine source directory.
	SourcePath string `yaml:"sourcePath"`
	// Version is the engine version tag (e.g. "5.7.3").
	Version string `yaml:"version"`
	// MaxJobs limits parallel compile jobs. 0 = auto-detect based on RAM.
	MaxJobs int `yaml:"maxJobs"`
	// Backend selects the build environment: "native" (default) or "docker".
	Backend string `yaml:"backend"`
	// DockerImage is a pre-built engine image URI (e.g. ECR URI). When set,
	// the engine build stage is skipped and game builds use this image directly.
	DockerImage string `yaml:"dockerImage"`
	// DockerImageName is the local Docker image name for engine builds (default: "ludus-engine").
	DockerImageName string `yaml:"dockerImageName"`
	// DockerBaseImage is the base Docker image for engine builds (default: "ubuntu:22.04").
	// Supports any Debian/Ubuntu (apt-get) or RHEL/Amazon Linux/Fedora (dnf) base.
	DockerBaseImage string `yaml:"dockerBaseImage"`
}

// GameConfig holds UE5 game project build settings.
type GameConfig struct {
	// ProjectPath is the path to the .uproject file.
	// For Lyra, if empty, defaults to <engine>/Samples/Games/Lyra/Lyra.uproject.
	ProjectPath string `yaml:"projectPath"`
	// ProjectName is the name of the UE5 project (e.g. "Lyra", "MyGame").
	ProjectName string `yaml:"projectName"`
	// ContentSourcePath is the path to the downloaded game content that needs to
	// be overlaid onto the engine source tree. For Lyra, this is the path to the
	// "Lyra Starter Game" downloaded from the Epic Games Launcher (e.g.
	// "C:\Users\...\Unreal Projects\LyraStarterGame"). When set, `ludus init --fix`
	// will copy Content/ and plugin Content/ directories into the engine's
	// Samples/Games/Lyra/ directory.
	ContentSourcePath string `yaml:"contentSourcePath"`
	// ServerTarget is the server build target name. Defaults to ProjectName + "Server".
	ServerTarget string `yaml:"serverTarget"`
	// ClientTarget is the client build target name. Defaults to ProjectName + "Game".
	ClientTarget string `yaml:"clientTarget"`
	// GameTarget is the default game target name. Defaults to ProjectName + "Game".
	GameTarget string `yaml:"gameTarget"`
	// Platform is the target build platform (default: "linux").
	Platform string `yaml:"platform"`
	// Arch is the target CPU architecture for server builds: "amd64" or "arm64".
	// Also accepts "x86_64" and "aarch64" (normalized to Go names).
	Arch string `yaml:"arch"`
	// SkipCook skips content cooking.
	SkipCook bool `yaml:"skipCook"`
	// ServerMap is the default map for the dedicated server.
	ServerMap string `yaml:"serverMap"`
	// ContentValidation configures how project content is validated during prereq checks.
	ContentValidation *ContentValidationConfig `yaml:"contentValidation,omitempty"`
}

// ContentValidationConfig controls project content validation in prereq checks.
type ContentValidationConfig struct {
	// Disabled skips content validation entirely.
	Disabled bool `yaml:"disabled"`
	// ContentMarkerFile is a file path relative to the project directory
	// used to verify content has been installed (e.g. "Content/DefaultGameData.uasset").
	ContentMarkerFile string `yaml:"contentMarkerFile"`
	// PluginContentDirs lists plugin subdirectories under Plugins/GameFeatures/
	// that must have a Content/ directory.
	PluginContentDirs []string `yaml:"pluginContentDirs"`
}

// ResolveProjectPath fills in ProjectPath if empty by checking known locations.
// For Lyra, it checks <engineSourcePath>/Samples/Games/Lyra/Lyra.uproject.
// For other projects, the user must set game.projectPath explicitly.
func (g *GameConfig) ResolveProjectPath(engineSourcePath string) {
	if g.ProjectPath != "" || engineSourcePath == "" {
		return
	}
	if g.ProjectName == "Lyra" || g.ProjectName == "" {
		candidate := filepath.Join(engineSourcePath, "Samples", "Games", "Lyra", "Lyra.uproject")
		if _, err := os.Stat(candidate); err == nil {
			g.ProjectPath = candidate
		}
	}
}

// ResolvedServerTarget returns the server target name, defaulting to ProjectName + "Server".
func (g *GameConfig) ResolvedServerTarget() string {
	if g.ServerTarget != "" {
		return g.ServerTarget
	}
	return g.ProjectName + "Server"
}

// ResolvedClientTarget returns the client target name, defaulting to ProjectName + "Game".
func (g *GameConfig) ResolvedClientTarget() string {
	if g.ClientTarget != "" {
		return g.ClientTarget
	}
	return g.ProjectName + "Game"
}

// ResolvedGameTarget returns the default game target name, defaulting to ProjectName + "Game".
func (g *GameConfig) ResolvedGameTarget() string {
	if g.GameTarget != "" {
		return g.GameTarget
	}
	return g.ProjectName + "Game"
}

// ResolvedArch returns the normalized architecture, defaulting to "amd64".
func (g *GameConfig) ResolvedArch() string {
	return NormalizeArch(g.Arch)
}

// NormalizeArch maps architecture aliases to Go's GOARCH values.
// Accepts: amd64, x86_64, arm64, aarch64 (case-insensitive). Defaults to "amd64".
func NormalizeArch(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "arm64", "aarch64":
		return "arm64"
	case "amd64", "x86_64", "":
		return "amd64"
	default:
		return "amd64"
	}
}

// ServerPlatformDir returns the UE output directory name for server builds.
// amd64 → "LinuxServer", arm64 → "LinuxArm64Server".
func ServerPlatformDir(arch string) string {
	if NormalizeArch(arch) == "arm64" {
		return "LinuxArm64Server"
	}
	return "LinuxServer"
}

// BinariesPlatformDir returns the UE Binaries subdirectory for the architecture.
// amd64 → "Linux", arm64 → "LinuxArm64".
func BinariesPlatformDir(arch string) string {
	if NormalizeArch(arch) == "arm64" {
		return "LinuxArm64"
	}
	return "Linux"
}

// UEPlatformName returns the UE platform name used in RunUAT -platform= flag.
// amd64 → "Linux", arm64 → "LinuxArm64".
func UEPlatformName(arch string) string {
	if NormalizeArch(arch) == "arm64" {
		return "LinuxArm64"
	}
	return "Linux"
}

// AnywhereConfig holds GameLift Anywhere settings for local development.
type AnywhereConfig struct {
	// LocationName is the custom location name (must start with "custom-"). Default: "custom-ludus-dev".
	LocationName string `yaml:"locationName"`
	// IPAddress is the local machine's IP address. Empty means auto-detect.
	IPAddress string `yaml:"ipAddress"`
	// AWSProfile is the AWS profile name for the wrapper's credential provider. Default: "default".
	AWSProfile string `yaml:"awsProfile"`
}

// DeployConfig holds deployment target settings.
type DeployConfig struct {
	// Target is the deployment backend: "gamelift" (default), "stack", "binary", "anywhere", or "ec2".
	Target string `yaml:"target"`
	// OutputDir is the output directory for the binary export target.
	OutputDir string `yaml:"outputDir"`
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

// EC2FleetConfig holds GameLift Managed EC2 fleet settings.
type EC2FleetConfig struct {
	// S3Bucket is the S3 bucket for server build uploads.
	// If empty, auto-creates "ludus-builds-<account-id>".
	S3Bucket string `yaml:"s3Bucket"`
	// ServerSDKVersion is the GameLift Server SDK version used by the wrapper.
	ServerSDKVersion string `yaml:"serverSdkVersion"`
}

// AWSConfig holds AWS account and region settings.
type AWSConfig struct {
	// Region is the AWS region for deployment.
	Region string `yaml:"region"`
	// AccountID is the AWS account ID (used for ECR URI construction).
	AccountID string `yaml:"accountId"`
	// ECRRepository is the ECR repository name.
	ECRRepository string `yaml:"ecrRepository"`
	// Tags are key-value pairs applied to all AWS resources created by Ludus.
	Tags map[string]string `yaml:"tags"`
}

// CIConfig holds CI workflow generation and self-hosted runner settings.
type CIConfig struct {
	// WorkflowPath is the output path for the generated workflow file.
	WorkflowPath string `yaml:"workflowPath"`
	// RunnerDir is the install directory for the GitHub Actions runner agent.
	RunnerDir string `yaml:"runnerDir"`
	// RunnerLabels are the labels applied to the self-hosted runner.
	RunnerLabels []string `yaml:"runnerLabels"`
}

// DDCConfig holds Derived Data Cache settings for UE5 builds.
type DDCConfig struct {
	Mode      string `yaml:"mode" mapstructure:"mode"`
	LocalPath string `yaml:"localPath" mapstructure:"localPath"`
}

// Clone returns a deep copy of the Config, ensuring nested maps, slices, and
// pointers are independently owned by the copy.
func (c *Config) Clone() Config {
	cp := *c

	cp.AWS.Tags = maps.Clone(c.AWS.Tags)
	cp.CI.RunnerLabels = slices.Clone(c.CI.RunnerLabels)

	if c.Game.ContentValidation != nil {
		cv := *c.Game.ContentValidation
		cv.PluginContentDirs = slices.Clone(c.Game.ContentValidation.PluginContentDirs)
		cp.Game.ContentValidation = &cv
	}

	return cp
}

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
	}
}

// ResolveServerBuildDir determines the packaged server build directory from config.
func ResolveServerBuildDir(cfg *Config) string {
	platformDir := ServerPlatformDir(cfg.Game.ResolvedArch())
	if cfg.Game.ProjectPath != "" {
		return filepath.Join(filepath.Dir(cfg.Game.ProjectPath), "PackagedServer", platformDir)
	}
	if cfg.Engine.SourcePath != "" && cfg.Game.ProjectName == "Lyra" {
		return filepath.Join(cfg.Engine.SourcePath, "Samples", "Games", "Lyra", "PackagedServer", platformDir)
	}
	return ""
}
