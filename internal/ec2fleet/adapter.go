package ec2fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/state"
)

// TargetAdapter wraps an EC2 fleet Deployer to implement deploy.Target and deploy.SessionManager.
type TargetAdapter struct {
	deployer *Deployer
}

// NewTargetAdapter creates a TargetAdapter wrapping the given Deployer.
func NewTargetAdapter(d *Deployer) *TargetAdapter {
	return &TargetAdapter{deployer: d}
}

// Deployer returns the underlying EC2 fleet Deployer for direct access.
func (a *TargetAdapter) Deployer() *Deployer {
	return a.deployer
}

func (a *TargetAdapter) Name() string { return "ec2" }

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

	serverBuildDir := input.ServerBuildDir
	if serverBuildDir == "" {
		return nil, fmt.Errorf("server build directory not provided")
	}

	// 1. Zip and upload to S3
	bucket, key, err := d.ZipAndUpload(ctx, serverBuildDir)
	if err != nil {
		return nil, err
	}

	// 2. Create GameLift Build
	buildID, err := d.CreateBuild(ctx, bucket, key)
	if err != nil {
		return nil, err
	}

	// 3. Create EC2 fleet
	fleetStatus, err := d.CreateFleet(ctx, buildID)
	if err != nil {
		return nil, err
	}

	// 4. Update state
	now := time.Now().UTC().Format(time.RFC3339)

	if err := state.UpdateFleet(&state.FleetState{
		FleetID:   fleetStatus.FleetID,
		Status:    fleetStatus.Status,
		CreatedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write fleet state: %v\n", err)
	}

	if err := state.UpdateEC2Fleet(&state.EC2FleetState{
		FleetID:   fleetStatus.FleetID,
		BuildID:   buildID,
		S3Bucket:  bucket,
		S3Key:     key,
		Status:    fleetStatus.Status,
		CreatedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write EC2 fleet state: %v\n", err)
	}

	if err := state.UpdateDeploy(&state.DeployState{
		TargetName: "ec2",
		Status:     fleetStatus.Status,
		Detail:     fmt.Sprintf("fleet %s, build %s", fleetStatus.FleetID, buildID),
		DeployedAt: now,
	}); err != nil {
		fmt.Printf("Warning: failed to write deploy state: %v\n", err)
	}

	return &deploy.DeployResult{
		TargetName: "ec2",
		Status:     fleetStatus.Status,
		Detail:     fmt.Sprintf("fleet %s, build %s", fleetStatus.FleetID, buildID),
	}, nil
}

func (a *TargetAdapter) Status(ctx context.Context) (*deploy.DeployStatus, error) {
	st, err := state.Load()
	if err != nil || st.EC2Fleet == nil {
		// Try looking up the fleet directly
		fleetStatus, err := a.deployer.GetFleetStatus(ctx)
		if err != nil {
			return &deploy.DeployStatus{
				TargetName: "ec2",
				Status:     "not_deployed",
				Detail:     "no EC2 fleet found",
			}, nil
		}
		return &deploy.DeployStatus{
			TargetName: "ec2",
			Status:     "active",
			Detail:     fmt.Sprintf("%s (%s)", fleetStatus.FleetID, fleetStatus.Status),
		}, nil
	}

	return &deploy.DeployStatus{
		TargetName: "ec2",
		Status:     "active",
		Detail:     fmt.Sprintf("fleet %s, build %s", st.EC2Fleet.FleetID, st.EC2Fleet.BuildID),
	}, nil
}

func (a *TargetAdapter) Destroy(ctx context.Context) error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	var fleetID, buildID, s3Bucket, s3Key string
	if st.EC2Fleet != nil {
		fleetID = st.EC2Fleet.FleetID
		buildID = st.EC2Fleet.BuildID
		s3Bucket = st.EC2Fleet.S3Bucket
		s3Key = st.EC2Fleet.S3Key
	}

	if fleetID == "" {
		// Try to find fleet by name
		fleetStatus, err := a.deployer.GetFleetStatus(ctx)
		if err == nil {
			fleetID = fleetStatus.FleetID
		}
	}

	if fleetID == "" && buildID == "" {
		fmt.Println("No EC2 fleet deployment state found.")
		return nil
	}

	if err := a.deployer.Destroy(ctx, fleetID, buildID, s3Bucket, s3Key); err != nil {
		return err
	}

	if err := state.ClearEC2Fleet(); err != nil {
		fmt.Printf("Warning: failed to clear EC2 fleet state: %v\n", err)
	}
	if err := state.ClearFleet(); err != nil {
		fmt.Printf("Warning: failed to clear fleet state: %v\n", err)
	}

	return nil
}

// CreateSession implements deploy.SessionManager.
func (a *TargetAdapter) CreateSession(ctx context.Context, maxPlayers int) (*deploy.SessionInfo, error) {
	// Get fleet ID from state or by name lookup
	var fleetID string
	st, err := state.Load()
	if err == nil && st.EC2Fleet != nil {
		fleetID = st.EC2Fleet.FleetID
	}
	if fleetID == "" {
		fleetStatus, err := a.deployer.GetFleetStatus(ctx)
		if err != nil {
			return nil, fmt.Errorf("finding fleet: %w", err)
		}
		fleetID = fleetStatus.FleetID
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
		fmt.Printf("Warning: failed to write session state: %v\n", err)
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
