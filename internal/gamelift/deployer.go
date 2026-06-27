package gamelift

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/jpvelasco/ludus/internal/awsutil"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/glsession"
	"github.com/jpvelasco/ludus/internal/tags"
)

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
	// Tags are applied to all AWS resources created by this deployer.
	Tags map[string]string
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
	opts      DeployOptions
	glClient  *gamelift.Client
	iamClient *iam.Client
}

// NewDeployer creates a new GameLift deployer using the provided AWS config.
func NewDeployer(opts DeployOptions, awsCfg aws.Config) *Deployer {
	return &Deployer{
		opts:      opts,
		glClient:  gamelift.NewFromConfig(awsCfg),
		iamClient: iam.NewFromConfig(awsCfg),
	}
}

// resourceTags returns the merged tag set for this deployer's resources.
func (d *Deployer) resourceTags() map[string]string {
	return tags.Merge(d.opts.Tags, map[string]string{
		"ludus:fleet-name": d.opts.FleetName,
	})
}

// CreateContainerGroupDefinition creates the container group definition in GameLift.
// If the definition already exists (e.g. from a prior partial deploy), reuses it.
func (d *Deployer) CreateContainerGroupDefinition(ctx context.Context) (string, error) {
	out, err := d.glClient.CreateContainerGroupDefinition(ctx, d.containerGroupDefinitionInput())
	if err != nil {
		if awsutil.IsConflict(err) {
			// Already exists (from partial failure or retry); describe + wait instead of failing.
			desc, derr := d.glClient.DescribeContainerGroupDefinition(ctx, &gamelift.DescribeContainerGroupDefinitionInput{
				Name: aws.String(d.opts.ContainerGroupName),
			})
			if derr == nil && desc.ContainerGroupDefinition != nil {
				arn := aws.ToString(desc.ContainerGroupDefinition.ContainerGroupDefinitionArn)
				if werr := d.waitForContainerGroupReady(ctx); werr != nil {
					return arn, werr
				}
				return arn, nil
			}
			// fall through to error if describe fails
		}
		return "", fmt.Errorf("creating container group definition: %w", err)
	}

	cgdARN := aws.ToString(out.ContainerGroupDefinition.ContainerGroupDefinitionArn)
	if err := d.waitForContainerGroupReady(ctx); err != nil {
		return cgdARN, err
	}
	return cgdARN, nil
}

// CreateFleet creates a new GameLift container fleet.
func (d *Deployer) CreateFleet(ctx context.Context, cgdARN string) (*FleetStatus, error) {
	roleARN, err := d.ensureIAMRole(ctx)
	if err != nil {
		return nil, err
	}

	input := buildCreateFleetInput(d.opts, roleARN, d.resourceTags())

	out, err := d.glClient.CreateContainerFleet(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("creating container fleet: %w", err)
	}

	fleetID := aws.ToString(out.ContainerFleet.FleetId)
	result := &FleetStatus{
		FleetID:              fleetID,
		FleetName:            d.opts.FleetName,
		InstanceType:         d.opts.InstanceType,
		ContainerGroupDefARN: cgdARN,
	}

	if err := d.waitForContainerFleetActive(ctx, fleetID, result); err != nil {
		return result, err
	}
	return result, nil
}

// CreateGameSession creates a test game session on the fleet.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (*deploy.SessionInfo, error) {
	return glsession.Create(ctx, d.glClient, fleetID, "", maxPlayers)
}

// DescribeGameSession returns the current status of a game session.
func (d *Deployer) DescribeGameSession(ctx context.Context, sessionID string) (string, error) {
	return glsession.Describe(ctx, d.glClient, sessionID)
}

// Destroy tears down all Ludus-managed AWS resources in reverse order:
// fleet → container group definition → IAM role.
func (d *Deployer) Destroy(ctx context.Context) error {
	if err := d.deleteFleet(ctx); err != nil {
		return err
	}
	if err := d.deleteContainerGroupDefinition(ctx); err != nil {
		return err
	}
	return d.deleteIAMRole(ctx)
}
