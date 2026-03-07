package gamelift

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/state"
)

// TargetAdapter wraps a GameLift Deployer to implement deploy.Target and deploy.SessionManager.
type TargetAdapter struct {
	deployer *Deployer
}

// NewTargetAdapter creates a TargetAdapter wrapping the given Deployer.
func NewTargetAdapter(d *Deployer) *TargetAdapter {
	return &TargetAdapter{deployer: d}
}

// Deployer returns the underlying GameLift Deployer for direct access
// by GameLift-specific commands (fleet, session).
func (a *TargetAdapter) Deployer() *Deployer {
	return a.deployer
}

func (a *TargetAdapter) Name() string { return "gamelift" }

func (a *TargetAdapter) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		NeedsContainerBuild: true,
		NeedsContainerPush:  true,
		SupportsSession:     true,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
}

func (a *TargetAdapter) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	fmt.Println("Creating container group definition...")
	cgdARN, err := a.deployer.CreateContainerGroupDefinition(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Container group definition ready: %s\n", cgdARN)

	fmt.Println("Creating fleet...")
	fleetStatus, err := a.deployer.CreateFleet(ctx, cgdARN)
	if err != nil {
		return nil, err
	}

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetStatus.FleetID,
		Status:    fleetStatus.Status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	return &deploy.DeployResult{
		TargetName: "gamelift",
		Status:     fleetStatus.Status,
		Detail:     fmt.Sprintf("fleet %s", fleetStatus.FleetID),
	}, nil
}

func (a *TargetAdapter) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	fleetStatus, err := a.deployer.GetFleetStatus(ctx)
	if err != nil {
		return &deploy.DeployStatus{
			TargetName: "gamelift",
			Status:     "not_deployed",
			Detail:     "no fleet found",
		}, nil
	}

	return &deploy.DeployStatus{
		TargetName: "gamelift",
		Status:     "active",
		Detail:     fmt.Sprintf("%s (%s)", fleetStatus.FleetID, fleetStatus.Status),
	}, nil
}

func (a *TargetAdapter) Destroy(ctx context.Context) error {
	if err := a.deployer.Destroy(ctx); err != nil {
		return err
	}

	if err := state.ClearFleet(); err != nil {
		fmt.Printf("Warning: failed to clear state: %v\n", err)
	}

	return nil
}

// CreateSession implements deploy.SessionManager.
func (a *TargetAdapter) CreateSession(ctx context.Context, maxPlayers int) (*deploy.SessionInfo, error) {
	fleetStatus, err := a.deployer.GetFleetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("finding fleet: %w", err)
	}

	info, err := a.deployer.CreateGameSession(ctx, fleetStatus.FleetID, maxPlayers)
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
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	return &deploy.SessionInfo{
		SessionID: info.SessionID,
		IPAddress: info.IPAddress,
		Port:      info.Port,
	}, nil
}

// DescribeSession implements deploy.SessionManager.
func (a *TargetAdapter) DescribeSession(ctx context.Context, sessionID string) (string, error) {
	return a.deployer.DescribeGameSession(ctx, sessionID)
}
