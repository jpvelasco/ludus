package gamelift

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/state"
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

// SetOutput routes the adapter's and the deployer's status prints to w
// (an MCP tool's per-operation capture). Implements deploy.OutputSetter
// (#765).
func (a *TargetAdapter) SetOutput(w io.Writer) { a.deployer.SetOutput(w) }

func (a *TargetAdapter) Name() string { return "gamelift" }

func (a *TargetAdapter) Capabilities() deploy.Capabilities {
	return deploy.Capabilities{
		NeedsContainerBuild: true,
		NeedsContainerPush:  true,
		NeedsAWS:            true,
		NeedsImageURI:       true,
		SupportsSession:     true,
		SupportsDeploy:      true,
		SupportsDestroy:     true,
	}
}

func (a *TargetAdapter) Deploy(ctx context.Context, input deploy.DeployInput) (*deploy.DeployResult, error) {
	fmt.Fprintln(a.deployer.output(), "Creating container group definition...")
	cgdARN, err := a.deployer.CreateContainerGroupDefinition(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(a.deployer.output(), "Container group definition ready: %s\n", cgdARN)

	fmt.Fprintln(a.deployer.output(), "Creating fleet...")
	fleetStatus, err := a.deployer.CreateFleet(ctx, cgdARN)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetStatus.FleetID,
		Status:    fleetStatus.Status,
		CreatedAt: now,
	}); err != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: failed to write fleet state: %v\n", err)
	}

	detail := fmt.Sprintf("fleet %s", fleetStatus.FleetID)
	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "gamelift",
		Status:     fleetStatus.Status,
		Detail:     detail,
		DeployedAt: now,
	}); err != nil {
		fmt.Fprintf(a.deployer.output(), "Warning: failed to write deploy state: %v\n", err)
	}

	return &deploy.DeployResult{
		TargetName: "gamelift",
		Status:     fleetStatus.Status,
		Detail:     detail,
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

	deploy.ClearFleetState()
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

	deploy.SaveSessionState(info)
	return info, nil
}

// DescribeSession implements deploy.SessionManager.
func (a *TargetAdapter) DescribeSession(ctx context.Context, sessionID string) (string, error) {
	return a.deployer.DescribeGameSession(ctx, sessionID)
}
