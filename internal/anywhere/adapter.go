package anywhere

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/state"
	"github.com/devrecon/ludus/internal/wrapper"
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

func (a *TargetAdapter) Name() string { return "anywhere" }

func (a *TargetAdapter) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		NeedsContainerBuild: false,
		NeedsContainerPush:  false,
		SupportsSession:     true,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
}

func (a *TargetAdapter) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	d := a.deployer
	opts := d.opts

	// 1. Detect local IP if not configured
	ipAddress := opts.IPAddress
	if ipAddress == "" {
		var err error
		ipAddress, err = DetectLocalIP()
		if err != nil {
			return nil, fmt.Errorf("auto-detecting local IP: %w", err)
		}
		fmt.Printf("Detected local IP: %s\n", ipAddress)
	}

	// 2. Ensure game server wrapper binary (anywhere always runs on the local machine's arch)
	fmt.Println("Ensuring game server wrapper binary...")
	wrapperBinary, err := wrapper.EnsureBinary(ctx, d.Runner, "")
	if err != nil {
		return nil, fmt.Errorf("game server wrapper: %w", err)
	}

	// 3. Create custom location
	fmt.Printf("Creating custom location %s...\n", opts.LocationName)
	locationARN, err := d.CreateLocation(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Location ready: %s\n", locationARN)

	// 4. Create Anywhere fleet
	fmt.Printf("Creating Anywhere fleet %s...\n", opts.FleetName)
	fleetID, fleetARN, err := d.CreateFleet(ctx, opts.LocationName)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Fleet created: %s\n", fleetID)

	// 5. Register this machine as a compute
	fmt.Println("Registering compute...")
	computeName, wsEndpoint, err := d.RegisterCompute(ctx, fleetID, opts.LocationName, ipAddress)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Compute registered: %s (endpoint: %s)\n", computeName, wsEndpoint)

	// 6. Launch server via wrapper
	fmt.Println("Launching game server...")
	pid, err := d.LaunchServer(ctx, wrapperBinary, fleetARN, locationARN, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("launching server: %w", err)
	}
	if pid > 0 {
		fmt.Printf("Server started (PID: %d)\n", pid)
	}

	// 7. Update state
	now := time.Now().UTC().Format(time.RFC3339)

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetID,
		Status:    "ACTIVE",
		CreatedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write fleet state: %v\n", err)
	}

	if err := state.UpdateAnywhere(&state.AnywhereState{
		PID:          pid,
		ComputeName:  computeName,
		FleetID:      fleetID,
		FleetARN:     fleetARN,
		LocationName: opts.LocationName,
		LocationARN:  locationARN,
		IPAddress:    ipAddress,
		ServerPort:   opts.ServerPort,
		StartedAt:    now,
	}); err != nil {
		fmt.Printf("Warning: failed to write anywhere state: %v\n", err)
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "anywhere",
		Status:     "active",
		Detail:     fmt.Sprintf("fleet %s, PID %d, %s:%d", fleetID, pid, ipAddress, opts.ServerPort),
		DeployedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write deploy state: %v\n", err)
	}

	return &deploy.DeployResult{
		TargetName: "anywhere",
		Status:     "active",
		Detail:     fmt.Sprintf("fleet %s, PID %d, %s:%d", fleetID, pid, ipAddress, opts.ServerPort),
	}, nil
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
		fmt.Println("No Anywhere deployment state found.")
		return nil
	}

	if err := a.deployer.Destroy(ctx, as.FleetID, as.ComputeName, as.LocationName, as.PID); err != nil {
		return err
	}

	if err := state.ClearAnywhere(); err != nil {
		fmt.Printf("Warning: failed to clear anywhere state: %v\n", err)
	}
	if err := state.ClearFleet(); err != nil {
		fmt.Printf("Warning: failed to clear fleet state: %v\n", err)
	}

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

	if err := state.UpdateSession(&state.SessionState{
		SessionID: info.SessionID,
		IPAddress: info.IPAddress,
		Port:      info.Port,
		Status:    "ACTIVE",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write session state: %v\n", err)
	}

	return info, nil
}

// DescribeSession implements deploy.SessionManager.
func (a *TargetAdapter) DescribeSession(ctx context.Context, sessionID string) (string, error) {
	return a.deployer.DescribeGameSession(ctx, sessionID)
}
