package stack

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"

	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/glsession"
	"github.com/devrecon/ludus/internal/tags"
)

const (
	pollInterval = 15 * time.Second
	maxPollWait  = 30 * time.Minute
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

	// Check if stack already exists
	existing, err := d.describeStack(ctx)
	if err == nil && existing != nil {
		return d.updateStack(ctx, templateBody, params, stackTags)
	}

	return d.createStack(ctx, templateBody, params, stackTags)
}

func (d *StackDeployer) createStack(ctx context.Context, templateBody string, params []cftypes.Parameter, stackTags []cftypes.Tag) (*StackResult, error) {
	fmt.Printf("Creating CloudFormation stack %q...\n", d.opts.StackName)

	out, err := d.cfnClient.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    aws.String(d.opts.StackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   params,
		Tags:         stackTags,
		Capabilities: []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam},
	})
	if err != nil {
		return nil, fmt.Errorf("creating stack: %w", err)
	}

	result := &StackResult{
		StackName: d.opts.StackName,
		StackID:   aws.ToString(out.StackId),
	}

	// Poll until complete
	status, err := d.pollStack(ctx, "CREATE_COMPLETE", "CREATE_FAILED", "ROLLBACK_COMPLETE")
	if err != nil {
		result.Status = status
		return result, err
	}

	result.Status = status
	result.FleetID = d.readFleetIDFromOutputs(ctx)
	return result, nil
}

func (d *StackDeployer) updateStack(ctx context.Context, templateBody string, params []cftypes.Parameter, stackTags []cftypes.Tag) (*StackResult, error) {
	fmt.Printf("Updating CloudFormation stack %q...\n", d.opts.StackName)

	_, err := d.cfnClient.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    aws.String(d.opts.StackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   params,
		Tags:         stackTags,
		Capabilities: []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam},
	})
	if err != nil {
		// "No updates are to be performed" is not an error
		if strings.Contains(err.Error(), "No updates are to be performed") {
			fmt.Println("Stack is already up to date.")
			status := d.readStackStatus(ctx)
			return &StackResult{
				StackName: d.opts.StackName,
				Status:    status,
				FleetID:   d.readFleetIDFromOutputs(ctx),
			}, nil
		}
		return nil, fmt.Errorf("updating stack: %w", err)
	}

	result := &StackResult{
		StackName: d.opts.StackName,
	}

	status, err := d.pollStack(ctx, "UPDATE_COMPLETE", "UPDATE_ROLLBACK_COMPLETE", "UPDATE_FAILED")
	if err != nil {
		result.Status = status
		return result, err
	}

	result.Status = status
	result.FleetID = d.readFleetIDFromOutputs(ctx)
	return result, nil
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

	_, err := d.cfnClient.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: aws.String(d.opts.StackName),
	})
	if err != nil {
		if isStackNotFound(err) {
			fmt.Println("Stack not found, skipping.")
			return nil
		}
		return fmt.Errorf("deleting stack: %w", err)
	}

	// Poll until DELETE_COMPLETE
	deadline := time.Now().Add(maxPollWait)
	for time.Now().Before(deadline) {
		stack, err := d.describeStack(ctx)
		if err != nil {
			if isStackNotFound(err) {
				fmt.Println("Stack deleted.")
				return nil
			}
			return fmt.Errorf("polling stack deletion: %w", err)
		}

		status := string(stack.StackStatus)
		fmt.Printf("  Stack status: %s\n", status)

		if status == "DELETE_COMPLETE" {
			fmt.Println("Stack deleted.")
			return nil
		}
		if status == "DELETE_FAILED" {
			reason := aws.ToString(stack.StackStatusReason)
			return fmt.Errorf("stack deletion failed: %s", reason)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return fmt.Errorf("timed out waiting for stack deletion")
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

func (d *StackDeployer) describeStack(ctx context.Context) (*cftypes.Stack, error) {
	out, err := d.cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(d.opts.StackName),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("stack %q not found", d.opts.StackName)
	}
	return &out.Stacks[0], nil
}

func (d *StackDeployer) pollStack(ctx context.Context, successStatus, failStatus, rollbackStatus string) (string, error) {
	deadline := time.Now().Add(maxPollWait)
	for time.Now().Before(deadline) {
		stack, err := d.describeStack(ctx)
		if err != nil {
			return "", fmt.Errorf("polling stack status: %w", err)
		}

		status := string(stack.StackStatus)
		fmt.Printf("  Stack status: %s\n", status)

		if status == successStatus {
			return status, nil
		}
		if status == failStatus || status == rollbackStatus {
			reason := aws.ToString(stack.StackStatusReason)
			return status, fmt.Errorf("stack operation failed (%s): %s", status, reason)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return "", fmt.Errorf("timed out waiting for stack to reach %s", successStatus)
}

func (d *StackDeployer) readFleetIDFromOutputs(ctx context.Context) string {
	status, err := d.Status(ctx)
	if err != nil {
		return ""
	}
	return status.FleetID
}

func (d *StackDeployer) readStackStatus(ctx context.Context) string {
	stack, err := d.describeStack(ctx)
	if err != nil {
		return "UNKNOWN"
	}
	return string(stack.StackStatus)
}

func isStackNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "NotFoundException") ||
		strings.Contains(msg, "NotFound")
}
