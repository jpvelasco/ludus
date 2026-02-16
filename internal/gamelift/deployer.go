package gamelift

import "context"

// DeployOptions configures the GameLift deployment.
type DeployOptions struct {
	// Region is the AWS region.
	Region string
	// ImageURI is the ECR image URI.
	ImageURI string
	// FleetName is the GameLift fleet name.
	FleetName string
	// InstanceType is the EC2 instance type.
	InstanceType string
	// ContainerGroupName is the container group definition name.
	ContainerGroupName string
	// ServerPort is the game server port.
	ServerPort int
	// ServerSDKVersion is the GameLift Server SDK version.
	ServerSDKVersion string
}

// FleetStatus represents the current state of a GameLift fleet.
type FleetStatus struct {
	FleetID              string `json:"fleetId"`
	FleetName            string `json:"fleetName"`
	Status               string `json:"status"`
	InstanceType         string `json:"instanceType"`
	ContainerGroupDefARN string `json:"containerGroupDefArn"`
}

// Deployer handles GameLift container fleet deployment.
type Deployer struct {
	opts DeployOptions
}

// NewDeployer creates a new GameLift deployer.
func NewDeployer(opts DeployOptions) *Deployer {
	return &Deployer{opts: opts}
}

// CreateContainerGroupDefinition creates the container group definition in GameLift.
func (d *Deployer) CreateContainerGroupDefinition(ctx context.Context) (string, error) {
	// TODO: Implement using AWS SDK for Go v2
	// 1. Call gamelift.CreateContainerGroupDefinition
	// 2. Configure port ranges, memory/CPU limits
	// 3. Wait for status to transition from COPYING to READY
	return "", nil
}

// CreateFleet creates a new GameLift container fleet.
func (d *Deployer) CreateFleet(ctx context.Context) (*FleetStatus, error) {
	// TODO: Implement using AWS SDK for Go v2
	// 1. Create IAM role with GameLiftContainerFleetPolicy
	// 2. Call gamelift.CreateContainerFleet
	// 3. Configure inbound permissions (UDP on server port)
	// 4. Wait for fleet to become ACTIVE
	return &FleetStatus{}, nil
}

// CreateGameSession creates a test game session on the fleet.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (string, error) {
	// TODO: Implement
	// Call gamelift.CreateGameSession with fleet ID and player count
	return "", nil
}

// GetFleetStatus returns the current status of the deployed fleet.
func (d *Deployer) GetFleetStatus(ctx context.Context) (*FleetStatus, error) {
	// TODO: Implement
	return &FleetStatus{}, nil
}
