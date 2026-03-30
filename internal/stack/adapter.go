package stack

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/state"
)

// TargetAdapter wraps a StackDeployer to implement deploy.Target and deploy.SessionManager.
type TargetAdapter struct {
	deployer *StackDeployer
}

// NewTargetAdapter creates a TargetAdapter wrapping the given StackDeployer.
func NewTargetAdapter(d *StackDeployer) *TargetAdapter {
	return &TargetAdapter{deployer: d}
}

// Deployer returns the underlying StackDeployer for direct access.
func (a *TargetAdapter) Deployer() *StackDeployer {
	return a.deployer
}

func (a *TargetAdapter) Name() string { return "stack" }

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
	result, err := a.deployer.Deploy(ctx)
	if err != nil {
		return nil, err
	}

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   result.FleetID,
		StackName: result.StackName,
		Status:    result.Status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write state: %v\n", err)
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "stack",
		Status:     result.Status,
		Detail:     fmt.Sprintf("stack %s, fleet %s", result.StackName, result.FleetID),
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("Warning: failed to write deploy state: %v\n", err)
	}

	return &deploy.DeployResult{
		TargetName: "stack",
		Status:     result.Status,
		Detail:     fmt.Sprintf("stack %s, fleet %s", result.StackName, result.FleetID),
	}, nil
}

func (a *TargetAdapter) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	stackStatus, err := a.deployer.Status(ctx)
	if err != nil {
		return &deploy.DeployStatus{
			TargetName: "stack",
			Status:     "not_deployed",
			Detail:     "no stack found",
		}, nil
	}

	return &deploy.DeployStatus{
		TargetName: "stack",
		Status:     "active",
		Detail:     fmt.Sprintf("%s (%s), fleet %s", stackStatus.StackName, stackStatus.Status, stackStatus.FleetID),
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
	fleetID, err := a.deployer.GetFleetID(ctx)
	if err != nil {
		return nil, fmt.Errorf("finding fleet from stack: %w", err)
	}

	info, err := a.deployer.CreateGameSession(ctx, fleetID, maxPlayers)
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

	return info, nil
}

// DescribeSession implements deploy.SessionManager.
func (a *TargetAdapter) DescribeSession(ctx context.Context, sessionID string) (string, error) {
	return a.deployer.DescribeGameSession(ctx, sessionID)
}
