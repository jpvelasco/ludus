package gamelift

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
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

// LoadAWSConfig loads the default AWS SDK configuration for the given region.
func LoadAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

const (
	iamRoleName  = "LudusGameLiftContainerFleetRole"
	iamPolicyARN = "arn:aws:iam::aws:policy/GameLiftContainerFleetPolicy"
	pollInterval = 15 * time.Second
	maxPollWait  = 30 * time.Minute
)

// CreateContainerGroupDefinition creates the container group definition in GameLift.
func (d *Deployer) CreateContainerGroupDefinition(ctx context.Context) (string, error) {
	sdkVersion := d.opts.ServerSDKVersion
	if sdkVersion == "" {
		sdkVersion = "5.2.0"
	}

	input := &gamelift.CreateContainerGroupDefinitionInput{
		Name:                      aws.String(d.opts.ContainerGroupName),
		OperatingSystem:           gltypes.ContainerOperatingSystemAmazonLinux2023,
		TotalMemoryLimitMebibytes: aws.Int32(1024),
		TotalVcpuLimit:            aws.Float64(1.0),
		GameServerContainerDefinition: &gltypes.GameServerContainerDefinitionInput{
			ContainerName:    aws.String("lyra-server"),
			ImageUri:         aws.String(d.opts.ImageURI),
			ServerSdkVersion: aws.String(sdkVersion),
			PortConfiguration: &gltypes.ContainerPortConfiguration{
				ContainerPortRanges: []gltypes.ContainerPortRange{
					{
						FromPort: aws.Int32(int32(d.opts.ServerPort)),
						ToPort:   aws.Int32(int32(d.opts.ServerPort)),
						Protocol: gltypes.IpProtocolUdp,
					},
				},
			},
		},
	}

	out, err := d.glClient.CreateContainerGroupDefinition(ctx, input)
	if err != nil {
		return "", fmt.Errorf("creating container group definition: %w", err)
	}

	cgdARN := aws.ToString(out.ContainerGroupDefinition.ContainerGroupDefinitionArn)

	// Poll until READY
	deadline := time.Now().Add(maxPollWait)
	for time.Now().Before(deadline) {
		desc, err := d.glClient.DescribeContainerGroupDefinition(ctx, &gamelift.DescribeContainerGroupDefinitionInput{
			Name: aws.String(d.opts.ContainerGroupName),
		})
		if err != nil {
			return cgdARN, fmt.Errorf("polling container group definition status: %w", err)
		}

		status := desc.ContainerGroupDefinition.Status
		fmt.Printf("  Container group definition status: %s\n", status)
		if status == gltypes.ContainerGroupDefinitionStatusReady {
			return cgdARN, nil
		}
		if status == gltypes.ContainerGroupDefinitionStatusFailed {
			reason := aws.ToString(desc.ContainerGroupDefinition.StatusReason)
			return cgdARN, fmt.Errorf("container group definition failed: %s", reason)
		}

		time.Sleep(pollInterval)
	}

	return cgdARN, fmt.Errorf("timed out waiting for container group definition to become READY")
}

// ensureIAMRole creates the GameLift fleet IAM role if it doesn't exist, returns the role ARN.
func (d *Deployer) ensureIAMRole(ctx context.Context) (string, error) {
	// Check if role already exists
	getOut, err := d.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err == nil {
		return aws.ToString(getOut.Role.Arn), nil
	}

	// Create the role
	assumeRolePolicy := `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Service": "gamelift.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }]
}`

	createOut, err := d.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(iamRoleName),
		AssumeRolePolicyDocument: aws.String(assumeRolePolicy),
		Description:              aws.String("IAM role for Ludus GameLift container fleet"),
	})
	if err != nil {
		return "", fmt.Errorf("creating IAM role: %w", err)
	}

	// Attach the GameLift policy
	_, err = d.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(iamRoleName),
		PolicyArn: aws.String(iamPolicyARN),
	})
	if err != nil {
		return "", fmt.Errorf("attaching policy to role: %w", err)
	}

	// Wait for IAM propagation
	time.Sleep(10 * time.Second)

	return aws.ToString(createOut.Role.Arn), nil
}

// CreateFleet creates a new GameLift container fleet.
func (d *Deployer) CreateFleet(ctx context.Context, cgdARN string) (*FleetStatus, error) {
	roleARN, err := d.ensureIAMRole(ctx)
	if err != nil {
		return nil, err
	}

	input := &gamelift.CreateContainerFleetInput{
		FleetRoleArn: aws.String(roleARN),
		Description:  aws.String("Ludus Lyra dedicated server fleet"),
		InstanceType: aws.String(d.opts.InstanceType),
		GameServerContainerGroupDefinitionName: aws.String(d.opts.ContainerGroupName),
		InstanceInboundPermissions: []gltypes.IpPermission{
			{
				FromPort: aws.Int32(int32(d.opts.ServerPort)),
				ToPort:   aws.Int32(int32(d.opts.ServerPort)),
				IpRange:  aws.String("0.0.0.0/0"),
				Protocol: gltypes.IpProtocolUdp,
			},
		},
	}

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

	// Poll until ACTIVE
	deadline := time.Now().Add(maxPollWait)
	for time.Now().Before(deadline) {
		desc, err := d.glClient.DescribeContainerFleet(ctx, &gamelift.DescribeContainerFleetInput{
			FleetId: aws.String(fleetID),
		})
		if err != nil {
			return result, fmt.Errorf("polling fleet status: %w", err)
		}

		status := desc.ContainerFleet.Status
		result.Status = string(status)
		fmt.Printf("  Fleet status: %s\n", status)

		if status == gltypes.ContainerFleetStatusActive {
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	return result, fmt.Errorf("timed out waiting for fleet to become ACTIVE")
}

// CreateGameSession creates a test game session on the fleet.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (string, error) {
	out, err := d.glClient.CreateGameSession(ctx, &gamelift.CreateGameSessionInput{
		FleetId:                  aws.String(fleetID),
		MaximumPlayerSessionCount: aws.Int32(int32(maxPlayers)),
	})
	if err != nil {
		return "", fmt.Errorf("creating game session: %w", err)
	}

	sessionID := aws.ToString(out.GameSession.GameSessionId)
	ip := aws.ToString(out.GameSession.IpAddress)
	port := aws.ToInt32(out.GameSession.Port)
	fmt.Printf("  Game session: %s\n  Connect: %s:%d\n", sessionID, ip, port)

	return sessionID, nil
}

// GetFleetStatus returns the current status of the deployed fleet.
func (d *Deployer) GetFleetStatus(ctx context.Context) (*FleetStatus, error) {
	out, err := d.glClient.ListContainerFleets(ctx, &gamelift.ListContainerFleetsInput{
		ContainerGroupDefinitionName: aws.String(d.opts.ContainerGroupName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing fleets: %w", err)
	}

	if len(out.ContainerFleets) == 0 {
		return nil, fmt.Errorf("no fleets found for container group %s", d.opts.ContainerGroupName)
	}

	fleet := out.ContainerFleets[0]
	return &FleetStatus{
		FleetID: aws.ToString(fleet.FleetId),
		Status:  string(fleet.Status),
	}, nil
}
