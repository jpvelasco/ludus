package config

// Config holds the full Ludus configuration, typically loaded from ludus.yaml.
type Config struct {
	Engine        EngineConfig        `yaml:"engine"`
	Game          GameConfig          `yaml:"game"`
	Container     ContainerConfig     `yaml:"container"`
	Deploy        DeployConfig        `yaml:"deploy"`
	GameLift      GameLiftConfig      `yaml:"gamelift"`
	EC2Fleet      EC2FleetConfig      `yaml:"ec2fleet"`
	Anywhere      AnywhereConfig      `yaml:"anywhere"`
	AWS           AWSConfig           `yaml:"aws"`
	CI            CIConfig            `yaml:"ci"`
	DDC           DDCConfig           `yaml:"ddc"`
	Observability ObservabilityConfig `yaml:"observability"`
	Privacy       PrivacyConfig       `yaml:"privacy"`
}

// PrivacyConfig controls masking of sensitive identifiers in terminal output.
type PrivacyConfig struct {
	// MaskAccountID masks the 12-digit AWS account ID in ECR URIs and ARNs in
	// human-readable output (default: true). JSON/MCP output is never masked.
	// Override per-invocation with the --show-account-id flag.
	MaskAccountID bool `yaml:"maskAccountId" mapstructure:"maskAccountId"`
}

// ObservabilityConfig holds build-observability settings: on-disk logs and
// optional OpenTelemetry (OTLP) trace export.
type ObservabilityConfig struct {
	Logs LogsConfig `yaml:"logs"`
	OTLP OTLPConfig `yaml:"otlp"`
}

// LogsConfig controls persisting build output to disk.
type LogsConfig struct {
	// Enabled is a tri-state: nil means default (on). Use IsEnabled() to read.
	Enabled *bool `yaml:"enabled"`
	// Dir is the log directory (default ".ludus/logs", project-local).
	Dir string `yaml:"dir"`
	// RetainRuns is how many run logs to keep before pruning oldest (default 20).
	RetainRuns int `yaml:"retainRuns"`
}

// IsEnabled reports whether on-disk logging is on. Absent config defaults to true.
func (l LogsConfig) IsEnabled() bool {
	return l.Enabled == nil || *l.Enabled
}

// OTLPConfig controls OpenTelemetry trace export. Disabled by default.
type OTLPConfig struct {
	Enabled  bool              `yaml:"enabled"`
	Endpoint string            `yaml:"endpoint"`
	Insecure bool              `yaml:"insecure"`
	Headers  map[string]string `yaml:"headers"`
}

// EngineConfig holds UE5 engine build settings.
type EngineConfig struct {
	// SourcePath is the path to the Unreal Engine source directory.
	SourcePath string `yaml:"sourcePath"`
	// Version is the engine version tag (e.g. "5.7.3").
	Version string `yaml:"version"`
	// MaxJobs limits parallel compile jobs. 0 = auto-detect based on RAM.
	MaxJobs int `yaml:"maxJobs"`
	// Backend selects the build environment: "native" (default), "docker", "podman", or "wsl2".
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
	// Mode selects the DDC backend: "zen" (default), "local" (legacy
	// FileSystem cache, deprecated), or "none".
	Mode string `yaml:"mode" mapstructure:"mode"`
	// LocalPath is the host directory for the legacy FileSystem DDC (mode
	// "local" only). Defaults to ~/.ludus/ddc.
	LocalPath string `yaml:"localPath" mapstructure:"localPath"`
	// ZenPath overrides the host directory for the UE5 Zen Store cache, UE's
	// default local DDC backend since 5.4. In container builds this path must
	// be persisted to get cook DDC reuse across --rm runs. Defaults to
	// ~/.ludus/zen when mode is "zen".
	ZenPath string `yaml:"zenPath" mapstructure:"zenPath"`
}
