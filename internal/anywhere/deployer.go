package anywhere

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/glsession"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/tags"
	"github.com/jpvelasco/ludus/internal/wrapperconfig"
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
	// PackagedDirName is the packaged content directory name (the .uproject
	// name, e.g. "LyraStarterGame6"). When empty, falls back to ProjectName.
	PackagedDirName string
	ServerTarget    string
	ServerMap       string
	AWSProfile      string
}

// packagedDirName returns the packaged content directory name, falling back to
// ProjectName when not explicitly set.
func (o DeployOptions) packagedDirName() string {
	if o.PackagedDirName != "" {
		return o.PackagedDirName
	}
	return o.ProjectName
}

// gameLiftAPI is the subset of the GameLift client the Anywhere deployer uses.
// Defining it as an interface lets tests inject a fake and assert which API
// calls a given operation makes (e.g. that rollback deletes the fleet but not a
// reused location). *gamelift.Client satisfies it.
type gameLiftAPI interface {
	CreateLocation(context.Context, *gamelift.CreateLocationInput, ...func(*gamelift.Options)) (*gamelift.CreateLocationOutput, error)
	CreateFleet(context.Context, *gamelift.CreateFleetInput, ...func(*gamelift.Options)) (*gamelift.CreateFleetOutput, error)
	RegisterCompute(context.Context, *gamelift.RegisterComputeInput, ...func(*gamelift.Options)) (*gamelift.RegisterComputeOutput, error)
	DeregisterCompute(context.Context, *gamelift.DeregisterComputeInput, ...func(*gamelift.Options)) (*gamelift.DeregisterComputeOutput, error)
	DescribeFleetAttributes(context.Context, *gamelift.DescribeFleetAttributesInput, ...func(*gamelift.Options)) (*gamelift.DescribeFleetAttributesOutput, error)
	DeleteFleet(context.Context, *gamelift.DeleteFleetInput, ...func(*gamelift.Options)) (*gamelift.DeleteFleetOutput, error)
	DeleteLocation(context.Context, *gamelift.DeleteLocationInput, ...func(*gamelift.Options)) (*gamelift.DeleteLocationOutput, error)
	CreateGameSession(context.Context, *gamelift.CreateGameSessionInput, ...func(*gamelift.Options)) (*gamelift.CreateGameSessionOutput, error)
	DescribeGameSessions(context.Context, *gamelift.DescribeGameSessionsInput, ...func(*gamelift.Options)) (*gamelift.DescribeGameSessionsOutput, error)
}

// Deployer handles GameLift Anywhere fleet operations.
type Deployer struct {
	opts     DeployOptions
	glClient gameLiftAPI
	Runner   *runner.Runner
	// outputWriter routes status prints away from the process stdout; nil
	// keeps the default CLI behavior.
	outputWriter io.Writer
}

// NewDeployer creates a new Anywhere deployer.
func NewDeployer(opts DeployOptions, awsCfg aws.Config, r *runner.Runner) *Deployer {
	return &Deployer{
		opts:     opts,
		glClient: gamelift.NewFromConfig(awsCfg),
		Runner:   r,
	}
}

// SetOutput routes this deployer's status prints and its runner's subprocess
// output to w instead of the process streams. The runner is constructed
// against the global streams before this deployer is wired (globals.NewRunner
// in target resolution), so the capture is re-derived here after
// construction. The MCP deploy tools call SetOutput with their per-operation
// capture; CLI and pipeline callers never call it (#765).
func (d *Deployer) SetOutput(w io.Writer) {
	d.outputWriter = w
	if d.Runner != nil {
		d.Runner.Stdout = w
		d.Runner.Stderr = w
	}
}

// output returns the writer status prints go to: the routed capture when
// SetOutput was called, else the process stdout.
func (d *Deployer) output() io.Writer {
	if d.outputWriter != nil {
		return d.outputWriter
	}
	return os.Stdout
}

// resourceTags returns the merged tag set for this deployer's resources.
func (d *Deployer) resourceTags() map[string]string {
	return tags.Merge(d.opts.Tags, map[string]string{
		"ludus:fleet-name": d.opts.FleetName,
		"ludus:target":     "anywhere",
	})
}

// gameLiftLocationName returns the location name as GameLift requires it:
// custom location names must start with "custom-". Normalizing here keeps
// location creation, fleet creation, compute registration, and persisted
// state all referring to the same resource — not just the create call.
func (o DeployOptions) gameLiftLocationName() string {
	if strings.HasPrefix(o.LocationName, "custom-") {
		return o.LocationName
	}
	return "custom-" + o.LocationName
}

// CreateLocation creates a custom GameLift location, tolerating conflicts
// (location already exists). The returned created flag reports whether this call
// actually created the location (false when an existing one was reused), so
// callers can avoid deleting a location they did not create during rollback.
func (d *Deployer) CreateLocation(ctx context.Context) (locationARN string, created bool, err error) {
	loc := d.opts.gameLiftLocationName()

	out, err := d.glClient.CreateLocation(ctx, &gamelift.CreateLocationInput{
		LocationName: aws.String(loc),
		Tags:         tags.ToGameLiftTags(d.resourceTags()),
	})
	if err != nil {
		// Tolerate ConflictException (location already exists)
		if awsutil.IsConflict(err) {
			fmt.Fprintf(d.output(), "  Location %s already exists, reusing.\n", loc)
			return fmt.Sprintf("arn:aws:gamelift:%s::location/%s", d.opts.Region, loc), false, nil
		}
		return "", false, fmt.Errorf("creating location: %w", err)
	}

	return aws.ToString(out.Location.LocationArn), true, nil
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

// serverExecutablePath returns the platform-appropriate path to the game server executable.
// On Windows the binary lives under Binaries/Win64 with a .exe suffix;
// on Linux it uses Binaries/Linux (or LinuxArm64 for arm64).
// The result always uses forward slashes: it is embedded in the wrapper's
// YAML config, where a backslash inside a quoted scalar is an escape character.
func serverExecutablePath(buildDir, packagedDir, platformDir, name string) string {
	// Replace every backslash, not just the OS separator: user-supplied
	// segments can carry Windows-style separators on any host, and a raw
	// backslash inside the YAML quoted scalar is an escape character.
	return strings.ReplaceAll(filepath.Join(buildDir, packagedDir, "Binaries", platformDir, name), `\`, `/`)
}

// hostServerPlatformDir returns the UE Binaries subdirectory and executable
// suffix for a server running on this machine. Anywhere fleets launch the
// wrapper on the host, so the platform is the host OS and architecture, not
// a cross-compile target.
func hostServerPlatformDir() (platformDir, exeSuffix string) {
	if runtime.GOOS == "windows" {
		return "Win64", ".exe"
	}
	if runtime.GOARCH == "arm64" {
		return "LinuxArm64", ""
	}
	return "Linux", ""
}

// resolveServerExecutablePath discovers the packaged server executable via
// the canonical discovery shared with the container and EC2 builders —
// Shipping/Test builds use the suffixed "<Target>-<Platform>-<Config>"
// filename — and validates that it exists on disk: the wrapper cannot start
// a config referencing a binary that is not there (#744).
func (d *Deployer) resolveServerExecutablePath() (string, error) {
	platformDir, exeSuffix := hostServerPlatformDir()
	name := config.DiscoverServerBinaryName(
		d.opts.ServerBuildDir,
		d.opts.packagedDirName(),
		d.opts.ServerTarget,
		platformDir,
		"",
		exeSuffix,
	)
	path := serverExecutablePath(d.opts.ServerBuildDir, d.opts.packagedDirName(), platformDir, name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("server executable not found at %s: package the server build before deploying Anywhere: %w", path, err)
	}
	return path, nil
}

// GenerateWrapperConfig produces the config.yaml for the GameLift Game Server Wrapper
// in Anywhere mode. executablePath must already be resolved (and validated to
// exist) via resolveServerExecutablePath.
func (d *Deployer) GenerateWrapperConfig(fleetARN, locationARN, executablePath, ipAddress string) string {
	return wrapperconfig.Config{
		Anywhere: &wrapperconfig.Anywhere{
			Profile:     d.opts.AWSProfile,
			LocationARN: locationARN,
			FleetARN:    fleetARN,
			IPv4:        ipAddress,
		},
		GamePort:       d.opts.ServerPort,
		ExecutablePath: executablePath,
		Args: []wrapperconfig.Arg{
			{Name: d.opts.ServerMap},
			wrapperconfig.PortArg(strconv.Itoa(d.opts.ServerPort)),
			{Name: "-log"},
		},
	}.YAML()
}

// LaunchServer writes the wrapper config into this deployment's runtime
// directory and starts the wrapper as a background process. It returns the
// PID of the wrapper process and the directory holding the config and server
// log, so callers can persist the path and later clean up exactly what this
// deployment created (#720).
func (d *Deployer) LaunchServer(ctx context.Context, wrapperBinary, executable, fleetARN, locationARN, ipAddress string) (int, string, error) {
	configDir, err := d.wrapperConfigDir()
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return 0, configDir, fmt.Errorf("creating wrapper config directory: %w", err)
	}

	configPath := filepath.Join(configDir, wrapperConfigFileName)
	configContent := d.GenerateWrapperConfig(fleetARN, locationARN, executable, ipAddress)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return 0, configDir, fmt.Errorf("writing wrapper config: %w", err)
	}

	if d.Runner.DryRun {
		fmt.Fprintf(d.output(), "+ %s -c %s (would launch wrapper)\n", wrapperBinary, configPath)
		return 0, configDir, nil
	}

	pid, err := launchProcess(wrapperBinary, configDir, configPath)
	return pid, configDir, err
}

// CreateGameSession creates a game session on the Anywhere fleet.
// The Location parameter is required for Anywhere fleets.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID, location string, maxPlayers int) (*deploy.SessionInfo, error) {
	return glsession.Create(ctx, d.glClient, fleetID, location, maxPlayers)
}

// DescribeGameSession returns the current status of a game session.
func (d *Deployer) DescribeGameSession(ctx context.Context, sessionID string) (string, error) {
	return glsession.Describe(ctx, d.glClient, sessionID)
}

// Destroy tears down Anywhere resources in reverse order:
// stop server → deregister compute → delete fleet → delete location, then
// removes this deployment's wrapper config directory when one is given (an
// empty configDir cleans nothing — the caller decides what this deployment
// owns, see resolveDestroyConfigDir).
func (d *Deployer) Destroy(ctx context.Context, fleetID, computeName, locationName string, pid int, configDir string) error {
	err := errors.Join(
		d.stopServerProcess(pid),
		d.deregisterComputeResource(ctx, fleetID, computeName),
		d.deleteFleetResource(ctx, fleetID),
		d.deleteLocationResource(ctx, locationName),
	)
	d.cleanupWrapperConfig(configDir)
	return err
}

// stopServerProcess kills the wrapper process if running.
func (d *Deployer) stopServerProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	fmt.Fprintln(d.output(), "Stopping server process...")
	if err := StopServer(pid); err != nil {
		return fmt.Errorf("stopping server (PID %d): %w", pid, err)
	}
	fmt.Fprintln(d.output(), "Server process stopped.")
	return nil
}

// deregisterComputeResource removes the compute from the fleet.
func (d *Deployer) deregisterComputeResource(ctx context.Context, fleetID, computeName string) error {
	if computeName == "" || fleetID == "" {
		return nil
	}
	fmt.Fprintln(d.output(), "Deregistering compute...")
	if err := d.DeregisterCompute(ctx, fleetID, computeName); err != nil {
		return fmt.Errorf("deregistering compute: %w", err)
	}
	fmt.Fprintln(d.output(), "Compute deregistered.")
	return nil
}

// deleteFleetResource deletes the Anywhere fleet.
func (d *Deployer) deleteFleetResource(ctx context.Context, fleetID string) error {
	if fleetID == "" {
		return nil
	}
	fmt.Fprintln(d.output(), "Deleting fleet...")
	_, err := d.glClient.DeleteFleet(ctx, &gamelift.DeleteFleetInput{
		FleetId: aws.String(fleetID),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("deleting fleet: %w", err)
	}
	fmt.Fprintln(d.output(), "Fleet deleted.")
	return nil
}

// deleteLocationResource deletes the custom location.
func (d *Deployer) deleteLocationResource(ctx context.Context, locationName string) error {
	if locationName == "" {
		return nil
	}
	fmt.Fprintln(d.output(), "Deleting location...")
	_, err := d.glClient.DeleteLocation(ctx, &gamelift.DeleteLocationInput{
		LocationName: aws.String(locationName),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("deleting location: %w", err)
	}
	fmt.Fprintln(d.output(), "Location deleted.")
	return nil
}

// cleanupWrapperConfig removes the given wrapper config directory,
// best-effort; an empty directory cleans nothing.
func (d *Deployer) cleanupWrapperConfig(configDir string) {
	if configDir == "" {
		return
	}
	if err := os.RemoveAll(configDir); err != nil {
		fmt.Fprintf(d.output(), "Warning: failed to remove wrapper config directory %s: %v\n", configDir, err)
	}
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

// wrapperConfigFileName is the wrapper config file inside a deployment's
// runtime directory.
const wrapperConfigFileName = "config.yaml"

// wrapperConfigDir returns the runtime directory for this deployment's
// wrapper config and server log, scoped per state profile and project so
// deployments never share — or delete — one another's files (#720). The
// result is deterministic for a profile+project pair, which lets rollback
// clean up a failed attempt without any recorded state.
func (d *Deployer) wrapperConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	scope := make([]string, 0, 2)
	for _, segment := range []string{state.ActiveProfile(), d.opts.ProjectName} {
		if segment != "" {
			scope = append(scope, sanitizeScopeSegment(segment))
		}
	}
	if len(scope) == 0 {
		scope = append(scope, "default")
	}
	return filepath.Join(append([]string{home, ".cache", "ludus", "anywhere"}, scope...)...), nil
}

// sanitizeScopeSegment makes a profile or project name safe to use as a
// single path segment, so a crafted name cannot escape the cache directory
// (or make RemoveAll walk outside it).
func sanitizeScopeSegment(segment string) string {
	var b strings.Builder
	for _, r := range segment {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// legacyWrapperConfigDir is the shared runtime directory used before config
// dirs were scoped per profile and project. Existing deployments recorded
// before then have no path in state; destroy falls back to this directory
// (only when its config file still exists) so they keep getting cleaned up.
func legacyWrapperConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "ludus", "anywhere"), nil
}

// wrapperCleanupDir decides which wrapper config directory a destroy must
// remove: exactly the path recorded at deploy time when the deployment state
// has one; else the legacy shared directory, but only when its config file
// still exists (a deployment recorded before per-deployment scoping); else
// nothing — never a directory another deployment may own (#720).
func wrapperCleanupDir(recorded, legacyDir string, legacyConfigExists bool) string {
	if recorded != "" {
		return recorded
	}
	if legacyConfigExists {
		return legacyDir
	}
	return ""
}

// resolveDestroyConfigDir computes the directory destroy must clean for a
// deployment whose state records recorded (empty for legacy deployments).
func resolveDestroyConfigDir(recorded string) string {
	legacy, err := legacyWrapperConfigDir()
	if err != nil {
		return recorded
	}
	_, statErr := os.Stat(filepath.Join(legacy, wrapperConfigFileName))
	return wrapperCleanupDir(recorded, legacy, statErr == nil)
}
