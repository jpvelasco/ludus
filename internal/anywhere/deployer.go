package anywhere

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/tags"
)

// DeployOptions configures an Anywhere deployment.
type DeployOptions struct {
	Region         string
	FleetName      string
	LocationName   string
	IPAddress      string
	ServerPort     int
	Tags           map[string]string
	ServerBuildDir string
	ProjectName    string
	ServerTarget   string
	ServerMap      string
	AWSProfile     string
}

// Deployer handles GameLift Anywhere fleet operations.
type Deployer struct {
	opts     DeployOptions
	glClient *gamelift.Client
	Runner   *runner.Runner
}

// NewDeployer creates a new Anywhere deployer.
func NewDeployer(opts DeployOptions, awsCfg aws.Config, r *runner.Runner) *Deployer {
	return &Deployer{
		opts:     opts,
		glClient: gamelift.NewFromConfig(awsCfg),
		Runner:   r,
	}
}

// resourceTags returns the merged tag set for this deployer's resources.
func (d *Deployer) resourceTags() map[string]string {
	return tags.Merge(d.opts.Tags, map[string]string{
		"ludus:fleet-name": d.opts.FleetName,
		"ludus:target":     "anywhere",
	})
}

// CreateLocation creates a custom GameLift location, tolerating conflicts
// (location already exists).
func (d *Deployer) CreateLocation(ctx context.Context) (string, error) {
	loc := d.opts.LocationName
	if !strings.HasPrefix(loc, "custom-") {
		loc = "custom-" + loc
	}

	out, err := d.glClient.CreateLocation(ctx, &gamelift.CreateLocationInput{
		LocationName: aws.String(loc),
		Tags:         tags.ToGameLiftTags(d.resourceTags()),
	})
	if err != nil {
		// Tolerate ConflictException (location already exists)
		if awsutil.IsConflict(err) {
			fmt.Printf("  Location %s already exists, reusing.\n", loc)
			return fmt.Sprintf("arn:aws:gamelift:%s::location/%s", d.opts.Region, loc), nil
		}
		return "", fmt.Errorf("creating location: %w", err)
	}

	locationARN := aws.ToString(out.Location.LocationArn)
	return locationARN, nil
}

// CreateFleet creates a GameLift Anywhere fleet and returns the fleet ID and ARN.
func (d *Deployer) CreateFleet(ctx context.Context, locationName string) (fleetID, fleetARN string, err error) {
	out, err := d.glClient.CreateFleet(ctx, &gamelift.CreateFleetInput{
		Name:        aws.String(d.opts.FleetName),
		Description: aws.String("Ludus Anywhere development fleet"),
		ComputeType: gltypes.ComputeTypeAnywhere,
		Locations: []gltypes.LocationConfiguration{
			{Location: aws.String(locationName)},
		},
		AnywhereConfiguration: &gltypes.AnywhereConfiguration{
			Cost: aws.String("1"),
		},
		Tags: tags.ToGameLiftTags(d.resourceTags()),
	})
	if err != nil {
		return "", "", fmt.Errorf("creating Anywhere fleet: %w", err)
	}

	fleetID = aws.ToString(out.FleetAttributes.FleetId)
	fleetARN = aws.ToString(out.FleetAttributes.FleetArn)
	return fleetID, fleetARN, nil
}

// RegisterCompute registers the local machine as a compute in the Anywhere fleet.
// Returns the compute name and WebSocket endpoint URL.
func (d *Deployer) RegisterCompute(ctx context.Context, fleetID, locationName, ipAddress string) (computeName, wsEndpoint string, err error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "ludus-dev"
	}
	computeName = fmt.Sprintf("ludus-%s", hostname)

	out, err := d.glClient.RegisterCompute(ctx, &gamelift.RegisterComputeInput{
		FleetId:     aws.String(fleetID),
		ComputeName: aws.String(computeName),
		IpAddress:   aws.String(ipAddress),
		Location:    aws.String(locationName),
	})
	if err != nil {
		return "", "", fmt.Errorf("registering compute: %w", err)
	}

	wsEndpoint = aws.ToString(out.Compute.GameLiftAgentEndpoint)
	return computeName, wsEndpoint, nil
}

// DeregisterCompute removes the compute from the fleet.
func (d *Deployer) DeregisterCompute(ctx context.Context, fleetID, computeName string) error {
	_, err := d.glClient.DeregisterCompute(ctx, &gamelift.DeregisterComputeInput{
		FleetId:     aws.String(fleetID),
		ComputeName: aws.String(computeName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("deregistering compute: %w", err)
	}
	return nil
}

// GetFleetStatus returns the current status of the Anywhere fleet.
func (d *Deployer) GetFleetStatus(ctx context.Context, fleetID string) (string, error) {
	out, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
		FleetIds: []string{fleetID},
	})
	if err != nil {
		return "", fmt.Errorf("describing fleet: %w", err)
	}
	if len(out.FleetAttributes) == 0 {
		return "", fmt.Errorf("fleet %s not found", fleetID)
	}
	return string(out.FleetAttributes[0].Status), nil
}

// serverBinaryPath returns the platform-appropriate path to the game server executable.
// On Windows the binary lives under Binaries/Win64 with a .exe suffix;
// on Linux it uses Binaries/Linux (or LinuxArm64 for arm64).
func serverBinaryPath(buildDir, projectName, serverTarget string) string {
	var platformDir, suffix string
	switch runtime.GOOS {
	case "windows":
		platformDir = "Win64"
		suffix = ".exe"
	default:
		if runtime.GOARCH == "arm64" {
			platformDir = "LinuxArm64"
		} else {
			platformDir = "Linux"
		}
	}
	return filepath.Join(buildDir, projectName, "Binaries", platformDir, serverTarget+suffix)
}

// GenerateWrapperConfig produces the config.yaml for the GameLift Game Server Wrapper
// in Anywhere mode.
func (d *Deployer) GenerateWrapperConfig(fleetARN, locationARN, wrapperBinary, ipAddress string) string {
	serverBinary := serverBinaryPath(d.opts.ServerBuildDir, d.opts.ProjectName, d.opts.ServerTarget)

	return fmt.Sprintf(`log-config:
  wrapper-log-level: info
anywhere:
  provider: aws-profile
  profile: %s
  location-arn: %s
  fleet-arn: %s
  ipv4: %s
ports:
  gamePort: %d
game-server-details:
  executable-file-path: %s
  game-server-args:
    - arg: "%s"
      val: ""
      pos: 0
    - arg: "-port="
      val: "%d"
      pos: 1
    - arg: "-log"
      val: ""
      pos: 2
`, d.opts.AWSProfile, locationARN, fleetARN, ipAddress,
		d.opts.ServerPort, serverBinary, d.opts.ServerMap, d.opts.ServerPort)
}

// LaunchServer writes the wrapper config and starts the wrapper as a background process.
// Returns the PID of the wrapper process.
func (d *Deployer) LaunchServer(ctx context.Context, wrapperBinary, fleetARN, locationARN, ipAddress string) (int, error) {
	// Write wrapper config next to the wrapper binary
	configDir, err := wrapperConfigDir()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return 0, fmt.Errorf("creating wrapper config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configContent := d.GenerateWrapperConfig(fleetARN, locationARN, wrapperBinary, ipAddress)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return 0, fmt.Errorf("writing wrapper config: %w", err)
	}

	if d.Runner.DryRun {
		fmt.Printf("+ %s (would launch wrapper from %s)\n", wrapperBinary, configDir)
		return 0, nil
	}

	return launchProcess(wrapperBinary, configDir)
}

// CreateGameSession creates a game session on the Anywhere fleet.
// The Location parameter is required for Anywhere fleets.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID, location string, maxPlayers int) (*deploy.SessionInfo, error) {
	out, err := d.glClient.CreateGameSession(ctx, &gamelift.CreateGameSessionInput{
		FleetId:                   aws.String(fleetID),
		Location:                  aws.String(location),
		MaximumPlayerSessionCount: aws.Int32(int32(maxPlayers)),
	})
	if err != nil {
		return nil, fmt.Errorf("creating game session: %w", err)
	}

	info := &deploy.SessionInfo{
		SessionID: aws.ToString(out.GameSession.GameSessionId),
		IPAddress: aws.ToString(out.GameSession.IpAddress),
		Port:      int(aws.ToInt32(out.GameSession.Port)),
	}
	fmt.Printf("  Game session: %s\n  Connect: %s:%d\n", info.SessionID, info.IPAddress, info.Port)
	return info, nil
}

// DescribeGameSession returns the current status of a game session.
func (d *Deployer) DescribeGameSession(ctx context.Context, sessionID string) (string, error) {
	out, err := d.glClient.DescribeGameSessions(ctx, &gamelift.DescribeGameSessionsInput{
		GameSessionId: aws.String(sessionID),
	})
	if err != nil {
		return "", fmt.Errorf("describing game session: %w", err)
	}
	if len(out.GameSessions) == 0 {
		return "", fmt.Errorf("game session %s not found", sessionID)
	}
	return string(out.GameSessions[0].Status), nil
}

// Destroy tears down Anywhere resources in reverse order:
// stop server → deregister compute → delete fleet → delete location.
func (d *Deployer) Destroy(ctx context.Context, fleetID, computeName, locationName string, pid int) error {
	// 1. Stop the server process
	if pid > 0 {
		fmt.Println("Stopping server process...")
		if err := StopServer(pid); err != nil {
			fmt.Printf("Warning: failed to stop server (PID %d): %v\n", pid, err)
		} else {
			fmt.Println("Server process stopped.")
		}
	}

	// 2. Deregister compute
	if computeName != "" && fleetID != "" {
		fmt.Println("Deregistering compute...")
		if err := d.DeregisterCompute(ctx, fleetID, computeName); err != nil {
			fmt.Printf("Warning: failed to deregister compute: %v\n", err)
		} else {
			fmt.Println("Compute deregistered.")
		}
	}

	// 3. Delete fleet
	if fleetID != "" {
		fmt.Println("Deleting fleet...")
		_, err := d.glClient.DeleteFleet(ctx, &gamelift.DeleteFleetInput{
			FleetId: aws.String(fleetID),
		})
		if err != nil && !awsutil.IsNotFound(err) {
			fmt.Printf("Warning: failed to delete fleet: %v\n", err)
		} else {
			fmt.Println("Fleet deleted.")
		}
	}

	// 4. Delete custom location
	if locationName != "" {
		fmt.Println("Deleting location...")
		_, err := d.glClient.DeleteLocation(ctx, &gamelift.DeleteLocationInput{
			LocationName: aws.String(locationName),
		})
		if err != nil && !awsutil.IsNotFound(err) {
			fmt.Printf("Warning: failed to delete location: %v\n", err)
		} else {
			fmt.Println("Location deleted.")
		}
	}

	// 5. Clean up wrapper config
	configDir, err := wrapperConfigDir()
	if err == nil {
		os.RemoveAll(configDir)
	}

	return nil
}

// DetectLocalIP returns the first non-loopback IPv4 address found on the machine.
func DetectLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("listing network interfaces: %w", err)
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		return ip.String(), nil
	}

	return "", fmt.Errorf("no non-loopback IPv4 address found")
}

// wrapperConfigDir returns the directory for Anywhere wrapper config files.
func wrapperConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "ludus", "anywhere"), nil
}
