package ec2fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/devrecon/ludus/internal/awsutil"
	"github.com/devrecon/ludus/internal/deploy"
	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/tags"
)

// DeployOptions configures the GameLift Managed EC2 deployment.
type DeployOptions struct {
	Region       string
	FleetName    string
	InstanceType string
	ServerPort   int
	S3Bucket     string // auto-create "ludus-builds-<account-id>" if empty
	ProjectName  string
	ServerTarget string
	ServerMap    string
	Arch         string // "amd64" (default) or "arm64"
	Tags         map[string]string
}

// FleetStatus represents the current state of a GameLift EC2 fleet.
type FleetStatus struct {
	FleetID  string
	BuildID  string
	Status   string
	S3Bucket string
	S3Key    string
}

// Deployer handles GameLift Managed EC2 fleet deployment.
type Deployer struct {
	opts      DeployOptions
	glClient  *gamelift.Client
	iamClient *iam.Client
	s3Client  *s3.Client
	stsClient *sts.Client
	Runner    *runner.Runner
}

// NewDeployer creates a new EC2 fleet deployer.
func NewDeployer(opts DeployOptions, awsCfg aws.Config, r *runner.Runner) *Deployer {
	return &Deployer{
		opts:      opts,
		glClient:  gamelift.NewFromConfig(awsCfg),
		iamClient: iam.NewFromConfig(awsCfg),
		s3Client:  s3.NewFromConfig(awsCfg),
		stsClient: sts.NewFromConfig(awsCfg),
		Runner:    r,
	}
}

const (
	iamRoleName  = "LudusGameLiftEC2FleetRole"
	iamPolicyARN = "arn:aws:iam::aws:policy/GameLiftContainerFleetPolicy"
	pollInterval = 15 * time.Second
	maxPollWait  = 30 * time.Minute
)

// resourceTags returns the merged tag set for this deployer's resources.
func (d *Deployer) resourceTags() map[string]string {
	return tags.Merge(d.opts.Tags, map[string]string{
		"ludus:fleet-name": d.opts.FleetName,
		"ludus:target":     "ec2",
	})
}

// resolveBucket determines the S3 bucket name, creating it if needed.
func (d *Deployer) resolveBucket(ctx context.Context) (string, error) {
	bucket := d.opts.S3Bucket
	if bucket != "" {
		return bucket, nil
	}

	// Auto-derive bucket name from AWS account ID
	identity, err := d.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("getting AWS account ID: %w", err)
	}
	accountID := aws.ToString(identity.Account)
	bucket = fmt.Sprintf("ludus-builds-%s", accountID)

	// Create bucket if it doesn't exist
	_, err = d.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		fmt.Printf("Creating S3 bucket %s...\n", bucket)
		createInput := &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		}
		// us-east-1 doesn't use LocationConstraint
		if d.opts.Region != "us-east-1" {
			createInput.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(d.opts.Region),
			}
		}
		if _, err := d.s3Client.CreateBucket(ctx, createInput); err != nil {
			return "", fmt.Errorf("creating S3 bucket: %w", err)
		}

		// Tag the bucket
		tagSet := tags.ToS3Tags(d.resourceTags())
		if len(tagSet) > 0 {
			_, _ = d.s3Client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
				Bucket: aws.String(bucket),
				Tagging: &s3types.Tagging{
					TagSet: tagSet,
				},
			})
		}
	}

	return bucket, nil
}

// ensureIAMRole creates the GameLift EC2 fleet IAM role if it doesn't exist.
func (d *Deployer) ensureIAMRole(ctx context.Context) (string, error) {
	getOut, err := d.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err == nil {
		return aws.ToString(getOut.Role.Arn), nil
	}

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
		Description:              aws.String("IAM role for Ludus GameLift EC2 fleet"),
		Tags:                     tags.ToIAMTags(d.resourceTags()),
	})
	if err != nil {
		return "", fmt.Errorf("creating IAM role: %w", err)
	}

	_, err = d.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(iamRoleName),
		PolicyArn: aws.String(iamPolicyARN),
	})
	if err != nil {
		return "", fmt.Errorf("attaching policy to role: %w", err)
	}

	// EC2 managed builds require S3 read access for GameLift to download
	// the build archive. The GameLiftContainerFleetPolicy only covers
	// container fleets, so we add an inline policy for S3.
	s3Policy := `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:GetObject", "s3:GetObjectVersion"],
    "Resource": "arn:aws:s3:::ludus-builds-*/*"
  }]
}`
	_, err = d.iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(iamRoleName),
		PolicyName:     aws.String("LudusS3BuildAccess"),
		PolicyDocument: aws.String(s3Policy),
	})
	if err != nil {
		return "", fmt.Errorf("adding S3 access policy: %w", err)
	}

	// Wait for IAM propagation
	time.Sleep(10 * time.Second)

	return aws.ToString(createOut.Role.Arn), nil
}

// CreateFleet creates a GameLift Managed EC2 fleet with the given build.
func (d *Deployer) CreateFleet(ctx context.Context, buildID string) (*FleetStatus, error) {
	roleARN, err := d.ensureIAMRole(ctx)
	if err != nil {
		return nil, err
	}

	// The wrapper binary is at the root of the zip; game server details
	// (executable path, map, etc.) are configured in config.yaml, not via
	// CLI flags. The wrapper only accepts --port to override the game port.
	launchPath := "/local/game/amazon-gamelift-servers-game-server-wrapper"
	launchParams := fmt.Sprintf("--port %d", d.opts.ServerPort)

	fmt.Println("Creating EC2 fleet...")
	out, err := d.glClient.CreateFleet(ctx, &gamelift.CreateFleetInput{
		Name:            aws.String(d.opts.FleetName),
		Description:     aws.String("Ludus dedicated server EC2 fleet"),
		BuildId:         aws.String(buildID),
		EC2InstanceType: gltypes.EC2InstanceType(d.opts.InstanceType),
		FleetType:       gltypes.FleetTypeOnDemand,
		InstanceRoleArn: aws.String(roleARN),
		RuntimeConfiguration: &gltypes.RuntimeConfiguration{
			ServerProcesses: []gltypes.ServerProcess{
				{
					LaunchPath:           aws.String(launchPath),
					Parameters:           aws.String(launchParams),
					ConcurrentExecutions: aws.Int32(1),
				},
			},
		},
		EC2InboundPermissions: []gltypes.IpPermission{
			{
				FromPort: aws.Int32(int32(d.opts.ServerPort)),
				ToPort:   aws.Int32(int32(d.opts.ServerPort)),
				IpRange:  aws.String("0.0.0.0/0"),
				Protocol: gltypes.IpProtocolUdp,
			},
		},
		Tags: tags.ToGameLiftTags(d.resourceTags()),
	})
	if err != nil {
		return nil, fmt.Errorf("creating EC2 fleet: %w", err)
	}

	fleetID := aws.ToString(out.FleetAttributes.FleetId)
	result := &FleetStatus{
		FleetID: fleetID,
		BuildID: buildID,
	}

	// Poll until ACTIVE
	deadline := time.Now().Add(maxPollWait)
	for time.Now().Before(deadline) {
		desc, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
			FleetIds: []string{fleetID},
		})
		if err != nil {
			return result, fmt.Errorf("polling fleet status: %w", err)
		}
		if len(desc.FleetAttributes) == 0 {
			return result, fmt.Errorf("fleet %s disappeared during polling", fleetID)
		}

		status := desc.FleetAttributes[0].Status
		result.Status = string(status)
		fmt.Printf("  Fleet status: %s\n", status)

		if status == gltypes.FleetStatusActive {
			return result, nil
		}
		if status == gltypes.FleetStatusError {
			return result, fmt.Errorf("fleet entered ERROR state")
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return result, fmt.Errorf("timed out waiting for fleet to become ACTIVE")
}

// CreateGameSession creates a game session on the EC2 fleet.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (*deploy.SessionInfo, error) {
	out, err := d.glClient.CreateGameSession(ctx, &gamelift.CreateGameSessionInput{
		FleetId:                   aws.String(fleetID),
		MaximumPlayerSessionCount: aws.Int32(int32(maxPlayers)),
	})
	if err != nil {
		return nil, fmt.Errorf("creating game session: %w", err)
	}

	info := &deploy.SessionInfo{
		SessionID: aws.ToString(out.GameSession.GameSessionId),
		IPAddress: aws.ToString(out.GameSession.IpAddress),
		Port:      int(aws.ToInt32(out.GameSession.Port)),
	}
	fmt.Printf("  Game session: %s\n  Connect: %s:%d\n", info.SessionID, info.IPAddress, info.Port)
	return info, nil
}

// DescribeGameSession returns the current status of a game session.
func (d *Deployer) DescribeGameSession(ctx context.Context, sessionID string) (string, error) {
	out, err := d.glClient.DescribeGameSessions(ctx, &gamelift.DescribeGameSessionsInput{
		GameSessionId: aws.String(sessionID),
	})
	if err != nil {
		return "", fmt.Errorf("describing game session: %w", err)
	}
	if len(out.GameSessions) == 0 {
		return "", fmt.Errorf("game session %s not found", sessionID)
	}
	return string(out.GameSessions[0].Status), nil
}

// GetFleetStatus looks up the fleet by name via ListFleets/DescribeFleetAttributes.
func (d *Deployer) GetFleetStatus(ctx context.Context) (*FleetStatus, error) {
	// List fleets with the build
	listOut, err := d.glClient.ListFleets(ctx, &gamelift.ListFleetsInput{})
	if err != nil {
		return nil, fmt.Errorf("listing fleets: %w", err)
	}

	if len(listOut.FleetIds) == 0 {
		return nil, fmt.Errorf("no fleets found")
	}

	// Describe to find our fleet by name
	descOut, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
		FleetIds: listOut.FleetIds,
	})
	if err != nil {
		return nil, fmt.Errorf("describing fleet attributes: %w", err)
	}

	for _, fleet := range descOut.FleetAttributes {
		if aws.ToString(fleet.Name) == d.opts.FleetName {
			return &FleetStatus{
				FleetID: aws.ToString(fleet.FleetId),
				Status:  string(fleet.Status),
			}, nil
		}
	}

	return nil, fmt.Errorf("no fleet found with name %s", d.opts.FleetName)
}

// Destroy tears down EC2 fleet resources in reverse order:
// fleet → build → S3 object → IAM role.
func (d *Deployer) Destroy(ctx context.Context, fleetID, buildID, s3Bucket, s3Key string) error {
	// 1. Delete fleet
	if fleetID != "" {
		fmt.Println("Deleting fleet...")
		_, err := d.glClient.DeleteFleet(ctx, &gamelift.DeleteFleetInput{
			FleetId: aws.String(fleetID),
		})
		if err != nil && !awsutil.IsNotFound(err) {
			return fmt.Errorf("deleting fleet: %w", err)
		}

		// Poll until the fleet is gone
		deadline := time.Now().Add(maxPollWait)
		for time.Now().Before(deadline) {
			desc, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
				FleetIds: []string{fleetID},
			})
			if err != nil {
				if awsutil.IsNotFound(err) {
					break
				}
				return fmt.Errorf("polling fleet deletion: %w", err)
			}
			if len(desc.FleetAttributes) == 0 {
				break
			}
			fmt.Println("  Waiting for fleet deletion...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
		fmt.Println("Fleet deleted.")
	}

	// 2. Delete build
	if buildID != "" {
		fmt.Println("Deleting build...")
		_, err := d.glClient.DeleteBuild(ctx, &gamelift.DeleteBuildInput{
			BuildId: aws.String(buildID),
		})
		if err != nil && !awsutil.IsNotFound(err) {
			fmt.Printf("Warning: failed to delete build: %v\n", err)
		} else {
			fmt.Println("Build deleted.")
		}
	}

	// 3. Delete S3 object
	if s3Bucket != "" && s3Key != "" {
		fmt.Printf("Deleting S3 object s3://%s/%s...\n", s3Bucket, s3Key)
		_, err := d.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s3Bucket),
			Key:    aws.String(s3Key),
		})
		if err != nil {
			fmt.Printf("Warning: failed to delete S3 object: %v\n", err)
		} else {
			fmt.Println("S3 object deleted.")
		}
	}

	// 4. Delete IAM role
	if err := d.deleteIAMRole(ctx); err != nil {
		fmt.Printf("Warning: failed to delete IAM role: %v\n", err)
	}

	return nil
}

func (d *Deployer) deleteIAMRole(ctx context.Context) error {
	fmt.Println("Deleting IAM role...")

	_, err := d.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		RoleName:  aws.String(iamRoleName),
		PolicyArn: aws.String(iamPolicyARN),
	})
	if err != nil && !awsutil.IsNotFound(err) {
		return fmt.Errorf("detaching policy from role: %w", err)
	}

	// Remove S3 access inline policy
	_, _ = d.iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(iamRoleName),
		PolicyName: aws.String("LudusS3BuildAccess"),
	})

	_, err = d.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			fmt.Println("IAM role not found, skipping.")
			return nil
		}
		return fmt.Errorf("deleting IAM role: %w", err)
	}

	fmt.Println("IAM role deleted.")
	return nil
}
