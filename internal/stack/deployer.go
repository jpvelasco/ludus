package stack

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"

	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/glsession"
	"github.com/devrecon/ludus/internal/tags"
)

// StackOptions configures a CloudFormation stack deployment.
type StackOptions struct {
	// StackName is the CloudFormation stack name.
	StackName string
	// Region is the AWS region.
	Region string
	// ImageURI is the ECR image URI for the game server container.
	ImageURI string
	// FleetName is the GameLift fleet name (used for tagging).
	FleetName string
	// InstanceType is the EC2 instance type for the fleet.
	InstanceType string
	// ContainerGroupName is the container group definition name.
	ContainerGroupName string
	// ServerPort is the game server port.
	ServerPort int
	// ServerSDKVersion is the GameLift Server SDK version.
	ServerSDKVersion string
	// Tags are applied to the stack and all its resources.
	Tags map[string]string
}

// StackResult holds the outcome of a stack deployment.
type StackResult struct {
	StackName string
	StackID   string
	Status    string
	FleetID   string
}

// StackStatus holds the current state of a CloudFormation stack.
type StackStatus struct {
	StackName string
	Status    string
	FleetID   string
	Outputs   map[string]string
}

// StackDeployer manages CloudFormation stack lifecycle for GameLift resources.
type StackDeployer struct {
	opts      StackOptions
	cfnClient *cloudformation.Client
	glClient  *gamelift.Client
}

// NewStackDeployer creates a new CloudFormation stack deployer.
func NewStackDeployer(opts StackOptions, awsCfg aws.Config) *StackDeployer {
	return &StackDeployer{
		opts:      opts,
		cfnClient: cloudformation.NewFromConfig(awsCfg),
		glClient:  gamelift.NewFromConfig(awsCfg),
	}
}

// Deploy creates or updates the CloudFormation stack and waits for completion.
func (d *StackDeployer) Deploy(ctx context.Context) (*StackResult, error) {
	templateBody := GenerateTemplate(TemplateOptions{
		ContainerGroupName: d.opts.ContainerGroupName,
		ServerPort:         d.opts.ServerPort,
		ServerSDKVersion:   d.opts.ServerSDKVersion,
		Tags:               d.stackResourceTags(),
	})

	stackTags := tags.ToCFNTags(d.stackResourceTags())

	params := []cftypes.Parameter{
		{ParameterKey: aws.String("ImageURI"), ParameterValue: aws.String(d.opts.ImageURI)},
		{ParameterKey: aws.String("ServerPort"), ParameterValue: aws.String(fmt.Sprintf("%d", d.opts.ServerPort))},
		{ParameterKey: aws.String("InstanceType"), ParameterValue: aws.String(d.opts.InstanceType)},
	}

	existing, err := d.describeStack(ctx)
	if err == nil && existing != nil {
		return d.updateStack(ctx, templateBody, params, stackTags)
	}

	return d.createStack(ctx, templateBody, params, stackTags)
}

// Status returns the current status of the CloudFormation stack.
func (d *StackDeployer) Status(ctx context.Context) (*StackStatus, error) {
	stack, err := d.describeStack(ctx)
	if err != nil {
		return nil, err
	}

	outputs := make(map[string]string, len(stack.Outputs))
	for _, o := range stack.Outputs {
		outputs[aws.ToString(o.OutputKey)] = aws.ToString(o.OutputValue)
	}

	return &StackStatus{
		StackName: d.opts.StackName,
		Status:    string(stack.StackStatus),
		FleetID:   outputs["FleetId"],
		Outputs:   outputs,
	}, nil
}

// Destroy deletes the CloudFormation stack and waits for completion.
func (d *StackDeployer) Destroy(ctx context.Context) error {
	fmt.Printf("Deleting CloudFormation stack %q...\n", d.opts.StackName)

	if err := d.deleteStack(ctx); err != nil {
		return err
	}
	return d.waitForStackDeletion(ctx)
}

// GetFleetID reads the FleetId output from the stack.
func (d *StackDeployer) GetFleetID(ctx context.Context) (string, error) {
	status, err := d.Status(ctx)
	if err != nil {
		return "", err
	}
	if status.FleetID == "" {
		return "", fmt.Errorf("FleetId not found in stack outputs")
	}
	return status.FleetID, nil
}

// CreateGameSession creates a game session on the fleet managed by this stack.
func (d *StackDeployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (*deploy.SessionInfo, error) {
	return glsession.Create(ctx, d.glClient, fleetID, "", maxPlayers)
}

// DescribeGameSession returns the current status of a game session.
func (d *StackDeployer) DescribeGameSession(ctx context.Context, sessionID string) (string, error) {
	return glsession.Describe(ctx, d.glClient, sessionID)
}

// stackResourceTags returns the merged tag set including fleet-name.
func (d *StackDeployer) stackResourceTags() map[string]string {
	return tags.Merge(d.opts.Tags, map[string]string{
		"ludus:fleet-name": d.opts.FleetName,
	})
}
