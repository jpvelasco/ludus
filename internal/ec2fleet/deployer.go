package ec2fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/glsession"
	"github.com/jpvelasco/ludus/internal/runner"
	"github.com/jpvelasco/ludus/internal/tags"
)

// DeployOptions configures the GameLift Managed EC2 deployment.
type DeployOptions struct {
	Region       string
	FleetName    string
	InstanceType string
	ServerPort   int
	S3Bucket     string // auto-create "ludus-builds-<account-id>" if empty
	ProjectName  string
	// PackagedDirName is the packaged content directory name (the .uproject
	// name, e.g. "LyraStarterGame6"). When empty, falls back to ProjectName.
	PackagedDirName string
	ServerTarget    string
	ServerMap       string
	Arch            string // "amd64" (default) or "arm64"
	Tags            map[string]string
}

// packagedDirName returns the packaged content directory name, falling back to
// ProjectName when not explicitly set.
func (o DeployOptions) packagedDirName() string {
	if o.PackagedDirName != "" {
		return o.PackagedDirName
	}
	return o.ProjectName
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

// CreateFleet creates a GameLift Managed EC2 fleet with the given build.
func (d *Deployer) CreateFleet(ctx context.Context, buildID string) (*FleetStatus, error) {
	roleARN, err := d.ensureIAMRole(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Println("Creating EC2 fleet...")
	out, err := d.glClient.CreateFleet(ctx, d.createFleetInput(buildID, roleARN))
	if err != nil {
		return nil, fmt.Errorf("creating EC2 fleet: %w", err)
	}

	fleetID := aws.ToString(out.FleetAttributes.FleetId)
	result := &FleetStatus{
		FleetID: fleetID,
		BuildID: buildID,
	}

	if err := d.waitForFleetActive(ctx, fleetID, result); err != nil {
		return result, err
	}
	return result, nil
}

// CreateGameSession creates a game session on the EC2 fleet.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (*deploy.SessionInfo, error) {
	return glsession.Create(ctx, d.glClient, fleetID, "", maxPlayers)
}

// DescribeGameSession returns the current status of a game session.
func (d *Deployer) DescribeGameSession(ctx context.Context, sessionID string) (string, error) {
	return glsession.Describe(ctx, d.glClient, sessionID)
}

// GetFleetStatus looks up the fleet by name via ListFleets/DescribeFleetAttributes.
func (d *Deployer) GetFleetStatus(ctx context.Context) (*FleetStatus, error) {
	listOut, err := d.glClient.ListFleets(ctx, &gamelift.ListFleetsInput{})
	if err != nil {
		return nil, fmt.Errorf("listing fleets: %w", err)
	}

	if len(listOut.FleetIds) == 0 {
		return nil, fmt.Errorf("no fleets found")
	}

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
	if err := d.deleteFleetResource(ctx, fleetID); err != nil {
		return err
	}
	d.deleteBuildResource(ctx, buildID)
	d.deleteS3Object(ctx, s3Bucket, s3Key)

	if err := d.deleteIAMRole(ctx); err != nil {
		fmt.Printf("Warning: failed to delete IAM role: %v\n", err)
	}

	return nil
}
