package anywhere

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/wrapper"
)

// TargetAdapter wraps an Anywhere Deployer to implement deploy.Target and deploy.SessionManager.
type TargetAdapter struct {
	deployer *Deployer
}

// NewTargetAdapter creates a TargetAdapter wrapping the given Deployer.
func NewTargetAdapter(d *Deployer) *TargetAdapter {
	return &TargetAdapter{deployer: d}
}

// Deployer returns the underlying Anywhere Deployer for direct access.
func (a *TargetAdapter) Deployer() *Deployer {
	return a.deployer
}

// SetOutput routes the adapter's and the deployer's status prints (and the
// runner's subprocess output) to w — an MCP tool's per-operation capture.
// Implements deploy.OutputSetter (#765).
func (a *TargetAdapter) SetOutput(w io.Writer) { a.deployer.SetOutput(w) }

func (a *TargetAdapter) Name() string { return "anywhere" }

func (a *TargetAdapter) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		NeedsContainerBuild: false,
		NeedsContainerPush:  false,
		NeedsAWS:            true,
		SupportsSession:     true,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
}

// anywhereResources holds the intermediate resources created during deployment.
type anywhereResources struct {
	ipAddress        string
	wrapperBinary    string
	locationARN      string
	fleetID          string
	fleetARN         string
	computeName      string
	pid              int
	wrapperConfigDir string
}

func (a *TargetAdapter) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	res, err := a.createResources(ctx)
	if err != nil {
		return nil, err
	}

	a.saveDeployState(res)

	detail := fmt.Sprintf("fleet %s, PID %d, %s:%d", res.fleetID, res.pid, res.ipAddress, a.deployer.opts.ServerPort)
	return &deploy.DeployResult{
		TargetName: "anywhere",
		Status:     "active",
		Detail:     detail,
	}, nil
}

// createResources provisions all Anywhere infrastructure: server executable
// validation, location, fleet, compute registration, and game server launch.
func (a *TargetAdapter) createResources(ctx context.Context) (*anywhereResources, error) {
	d := a.deployer
	opts := d.opts

	ipAddress, err := a.resolveIPAddress(opts.IPAddress)
	if err != nil {
		return nil, err
	}

	// Resolve and validate the packaged server executable before creating any
	// cloud resources: a config pointing at a nonexistent binary cannot start,
	// and failing here leaves nothing to roll back (#744).
	executable, err := d.resolveServerExecutablePath()
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(a.deployer.output(), "Ensuring game server wrapper binary...")
	wrapperBinary, err := wrapper.EnsureBinary(ctx, d.Runner, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, fmt.Errorf("game server wrapper: %w", err)
	}

	fmt.Fprintf(a.deployer.output(), "Creating custom location %s...\n", opts.LocationName)
	locationARN, locationCreated, err := d.CreateLocation(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(a.deployer.output(), "Location ready: %s\n", locationARN)

	fleetID, fleetARN, computeName, err := a.registerFleetAndCompute(ctx, ipAddress)
	if err != nil {
		// A failure after location creation must not orphan the location or
		// fleet: no state has been saved yet, so deploy destroy could not
		// find them (#719). Nothing was launched yet, so there is no runtime
		// directory of this attempt to clean.
		a.rollbackPartialResources(ctx, fleetID, computeName, locationCreated, "")
		return nil, err
	}

	fmt.Fprintln(a.deployer.output(), "Launching game server...")
	pid, configDir, err := d.LaunchServer(ctx, wrapperBinary, executable, fleetARN, locationARN, ipAddress)
	if err != nil {
		a.rollbackPartialResources(ctx, fleetID, computeName, locationCreated, configDir)
		return nil, fmt.Errorf("launching server: %w", err)
	}
	if pid > 0 {
		fmt.Fprintf(a.deployer.output(), "Server started (PID: %d)\n", pid)
	}

	return &anywhereResources{
		ipAddress:        ipAddress,
		wrapperBinary:    wrapperBinary,
		locationARN:      locationARN,
		fleetID:          fleetID,
		fleetARN:         fleetARN,
		computeName:      computeName,
		pid:              pid,
		wrapperConfigDir: configDir,
	}, nil
}

// rollbackPartialResources tears down resources created before a failed
// compute registration or LaunchServer, so a failed deploy does not orphan
// GameLift resources (no state has been saved yet, so `deploy destroy` could
// not find them). The location is only deleted when this attempt created it —
// CreateLocation reuses a pre-existing location on conflict, and we must not
// delete one we did not own — and it is named exactly as GameLift requires
// it: the normalized custom- name CreateLocation created (#719). configDir is
// this attempt's runtime directory when the launch step was reached, so the
// cleanup only removes what this attempt wrote.
func (a *TargetAdapter) rollbackPartialResources(ctx context.Context, fleetID, computeName string, locationCreated bool, configDir string) {
	fmt.Fprintln(a.deployer.output(), "Deployment failed; rolling back Anywhere resources...")
	rollbackLocation := ""
	if locationCreated {
		rollbackLocation = a.deployer.opts.gameLiftLocationName()
	}
	if derr := a.deployer.Destroy(ctx, fleetID, computeName, rollbackLocation, 0, configDir); derr != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: rollback incomplete: %v\n", derr)
	}
}

// resolveIPAddress returns the configured IP or auto-detects it, reporting a
// detection through the routed output writer (#765).
func (a *TargetAdapter) resolveIPAddress(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	ip, err := DetectLocalIP()
	if err != nil {
		return "", fmt.Errorf("auto-detecting local IP: %w", err)
	}
	fmt.Fprintf(a.deployer.output(), "Detected local IP: %s\n", ip)
	return ip, nil
}

// registerFleetAndCompute creates the Anywhere fleet and registers this machine.
func (a *TargetAdapter) registerFleetAndCompute(ctx context.Context, ipAddress string) (fleetID, fleetARN, computeName string, err error) {
	d := a.deployer
	opts := d.opts

	fmt.Fprintf(a.deployer.output(), "Creating Anywhere fleet %s...\n", opts.FleetName)
	fleetID, fleetARN, err = d.CreateFleet(ctx, opts.gameLiftLocationName())
	if err != nil {
		return fleetID, fleetARN, "", err
	}
	fmt.Fprintf(a.deployer.output(), "Fleet created: %s\n", fleetID)

	fmt.Fprintln(a.deployer.output(), "Registering compute...")
	computeName, wsEndpoint, err := d.RegisterCompute(ctx, fleetID, opts.gameLiftLocationName(), ipAddress)
	if err != nil {
		// Preserve the created fleet identifiers so the caller can roll back
		// instead of orphaning them.
		return fleetID, fleetARN, "", err
	}
	fmt.Fprintf(a.deployer.output(), "Compute registered: %s (endpoint: %s)\n", computeName, wsEndpoint)
	return fleetID, fleetARN, computeName, nil
}

// saveDeployState persists fleet, anywhere, and deploy state, logging warnings on failure.
func (a *TargetAdapter) saveDeployState(res *anywhereResources) {
	opts := a.deployer.opts
	now := time.Now().UTC().Format(time.RFC3339)
	detail := fmt.Sprintf("fleet %s, PID %d, %s:%d", res.fleetID, res.pid, res.ipAddress, opts.ServerPort)

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   res.fleetID,
		Status:    "ACTIVE",
		CreatedAt: now,
	}); err != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: failed to write fleet state: %v\n", err)
	}

	if err := state.UpdateAnywhere(&state.AnywhereState{
		PID:              res.pid,
		ComputeName:      res.computeName,
		FleetID:          res.fleetID,
		FleetARN:         res.fleetARN,
		LocationName:     opts.gameLiftLocationName(),
		LocationARN:      res.locationARN,
		IPAddress:        res.ipAddress,
		ServerPort:       opts.ServerPort,
		StartedAt:        now,
		WrapperConfigDir: res.wrapperConfigDir,
	}); err != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: failed to write anywhere state: %v\n", err)
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "anywhere",
		Status:     "active",
		Detail:     detail,
		DeployedAt: now,
	}); err != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: failed to write deploy state: %v\n", err)
	}
}

func (a *TargetAdapter) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	st, err := state.Load()
	if err != nil || st.Anywhere == nil {
		return &deploy.DeployStatus{
			TargetName: "anywhere",
			Status:     "not_deployed",
			Detail:     "no Anywhere deployment found",
		}, nil
	}

	as := st.Anywhere
	alive := IsProcessAlive(as.PID)

	if !alive {
		return &deploy.DeployStatus{
			TargetName: "anywhere",
			Status:     "not_deployed",
			Detail:     fmt.Sprintf("server process (PID %d) is not running", as.PID),
		}, nil
	}

	// Optionally verify fleet still exists
	status, err := a.deployer.GetFleetStatus(ctx, as.FleetID)
	if err != nil {
		return &deploy.DeployStatus{
			TargetName: "anywhere",
			Status:     "not_deployed",
			Detail:     fmt.Sprintf("fleet %s not found", as.FleetID),
		}, nil
	}

	return &deploy.DeployStatus{
		TargetName: "anywhere",
		Status:     "active",
		Detail:     fmt.Sprintf("fleet %s (%s), PID %d, %s:%d", as.FleetID, status, as.PID, as.IPAddress, as.ServerPort),
	}, nil
}

func (a *TargetAdapter) Destroy(ctx context.Context) error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	as := st.Anywhere
	if as == nil {
		fmt.Fprintln(a.deployer.output(), "No Anywhere deployment state found.")
		return nil
	}

	// Clean exactly what this deployment created: the recorded runtime
	// directory, or the legacy shared one for deployments recorded before
	// per-deployment scoping (#720).
	configDir := resolveDestroyConfigDir(as.WrapperConfigDir)
	if err := a.deployer.Destroy(ctx, as.FleetID, as.ComputeName, as.LocationName, as.PID, configDir); err != nil {
		return err
	}

	if err := state.ClearAnywhere(); err != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: failed to clear anywhere state: %v\n", err)
	}
	deploy.ClearFleetState()

	return nil
}

// CreateSession implements deploy.SessionManager.
func (a *TargetAdapter) CreateSession(ctx context.Context, maxPlayers int) (*deploy.SessionInfo, error) {
	st, err := state.Load()
	if err != nil {
		return nil, fmt.Errorf("loading state: %w", err)
	}

	as := st.Anywhere
	if as == nil {
		return nil, fmt.Errorf("no Anywhere deployment found; run 'ludus deploy anywhere' first")
	}

	info, err := a.deployer.CreateGameSession(ctx, as.FleetID, as.LocationName, maxPlayers)
	if err != nil {
		return nil, err
	}

	deploy.SaveSessionState(info)
	return info, nil
}

// DescribeSession implements deploy.SessionManager.
func (a *TargetAdapter) DescribeSession(ctx context.Context, sessionID string) (string, error) {
	return a.deployer.DescribeGameSession(ctx, sessionID)
}
