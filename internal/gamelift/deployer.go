// Package gamelift implements the deploy.Target adapter and supporting
// logic for AWS GameLift container fleets (including container group
// definitions, fleet creation, and IAM roles).
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
	"github.com/jpvelasco/ludus/internal/retry"
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
	opts                 DeployOptions
	glClient             *gamelift.Client
	cgdClient            containerGroupDefinitionClient
	cgdCreateRetryConfig retry.Config
	iamClient            *iam.Client
}

type containerGroupDefinitionClient interface {
	CreateContainerGroupDefinition(context.Context, *gamelift.CreateContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.CreateContainerGroupDefinitionOutput, error)
	DescribeContainerGroupDefinition(context.Context, *gamelift.DescribeContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.DescribeContainerGroupDefinitionOutput, error)
	DeleteContainerGroupDefinition(context.Context, *gamelift.DeleteContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.DeleteContainerGroupDefinitionOutput, error)
}

// NewDeployer creates a new GameLift deployer using the provided AWS config.
func NewDeployer(opts DeployOptions, awsCfg aws.Config) *Deployer {
	glClient := gamelift.NewFromConfig(awsCfg)
	return &Deployer{
		opts:                 opts,
		glClient:             glClient,
		cgdClient:            glClient,
		cgdCreateRetryConfig: retry.Default(),
		iamClient:            iam.NewFromConfig(awsCfg),
	}
}

// resourceTags returns the merged tag set for this deployer's resources.
func (d *Deployer) resourceTags() map[string]string {
	return tags.Merge(d.opts.Tags, map[string]string{
		"ludus:fleet-name": d.opts.FleetName,
	})
}

// CreateContainerGroupDefinition creates the container group definition in GameLift.
// Reuses on conflict if the existing definition matches the desired input exactly.
// On mismatch (e.g. different image after re-push of :latest), removes the stale definition
// and creates fresh. This provides better idempotency without leaving stale snapshots.
func (d *Deployer) CreateContainerGroupDefinition(ctx context.Context) (string, error) {
	input := d.containerGroupDefinitionInput()
	var arn string
	var fatalErr error
	retryErr := retry.Do(ctx, d.cgdCreateRetryConfig, func() error {
		var retryable bool
		arn, retryable, fatalErr = d.createContainerGroupDefinitionAttempt(ctx, input)
		if fatalErr == nil || !retryable {
			return nil
		}
		return fatalErr
	})
	if retryErr != nil {
		return "", fmt.Errorf("creating container group definition after retries: %w", retryErr)
	}
	if fatalErr != nil {
		return "", fatalErr
	}
	return d.waitAndReturn(ctx, arn)
}

func (d *Deployer) createContainerGroupDefinitionAttempt(ctx context.Context, input *gamelift.CreateContainerGroupDefinitionInput) (arn string, retryable bool, err error) {
	out, err := d.cgdClient.CreateContainerGroupDefinition(ctx, input)
	if err == nil {
		return aws.ToString(out.ContainerGroupDefinition.ContainerGroupDefinitionArn), false, nil
	}
	if !awsutil.IsConflict(err) {
		return "", false, fmt.Errorf("creating container group definition: %w", err)
	}
	return d.handleConflictOnCreate(ctx, err, input)
}

// waitAndReturn waits for the container group definition to be ready and returns its ARN.
func (d *Deployer) waitAndReturn(ctx context.Context, arn string) (string, error) {
	if werr := d.waitForContainerGroupReady(ctx); werr != nil {
		return arn, werr
	}
	return arn, nil
}

// handleConflictOnCreate describes the existing same-named definition. A
// matching definition is reused, a mismatch is deleted before retrying, and an
// inconclusive describe is retried because GameLift may still be deleting the
// definition that caused the conflict.
func (d *Deployer) handleConflictOnCreate(ctx context.Context, createErr error, input *gamelift.CreateContainerGroupDefinitionInput) (arn string, retryable bool, err error) {
	desc, derr := d.cgdClient.DescribeContainerGroupDefinition(ctx, &gamelift.DescribeContainerGroupDefinitionInput{
		Name: aws.String(d.opts.ContainerGroupName),
	})
	if derr != nil {
		return "", true, fmt.Errorf("creating container group definition: conflict on create but describe failed: %w (create err: %v)", derr, createErr)
	}
	if desc.ContainerGroupDefinition == nil {
		return "", true, fmt.Errorf("creating container group definition: conflict on create but describe returned no definition: %w", createErr)
	}
	if definitionMatches(desc.ContainerGroupDefinition, input) {
		return aws.ToString(desc.ContainerGroupDefinition.ContainerGroupDefinitionArn), false, nil
	}
	// Mismatch: remove the stale one so the next create attempt can succeed.
	_, delErr := d.cgdClient.DeleteContainerGroupDefinition(ctx, &gamelift.DeleteContainerGroupDefinitionInput{
		Name: aws.String(d.opts.ContainerGroupName),
	})
	if delErr != nil && !awsutil.IsNotFound(delErr) {
		return "", false, fmt.Errorf("existing container group definition does not match desired config and could not be removed: %w", delErr)
	}
	return "", true, fmt.Errorf("retrying create after removing stale container group definition: %w", createErr)
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
