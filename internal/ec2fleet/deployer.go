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
	iamClient iamAPI
	s3Client  *s3.Client
	stsClient *sts.Client
	Runner    *runner.Runner
}

//nolint:dupl // method-set seam mirroring the concrete *iam.Client; kept explicit for test fakes
type iamAPI interface {
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	AttachRolePolicy(context.Context, *iam.AttachRolePolicyInput, ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	DetachRolePolicy(context.Context, *iam.DetachRolePolicyInput, ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
	PutRolePolicy(context.Context, *iam.PutRolePolicyInput, ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
	DeleteRolePolicy(context.Context, *iam.DeleteRolePolicyInput, ...func(*iam.Options)) (*iam.DeleteRolePolicyOutput, error)
	ListAttachedRolePolicies(context.Context, *iam.ListAttachedRolePoliciesInput, ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
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

// fleetAPI is the subset of GameLift operations needed for name-based fleet
// lookup.
type fleetAPI interface {
	ListFleets(ctx context.Context, params *gamelift.ListFleetsInput, optFns ...func(*gamelift.Options)) (*gamelift.ListFleetsOutput, error)
	DescribeFleetAttributes(ctx context.Context, params *gamelift.DescribeFleetAttributesInput, optFns ...func(*gamelift.Options)) (*gamelift.DescribeFleetAttributesOutput, error)
}

// findFleetByName resolves a fleet by following ListFleets pagination
// (16 fleet IDs per page) and describing each page with its own NextToken.
// The previous single-page lookup missed fleets on page 2+ once an account
// accumulated more than 16 fleets.
func findFleetByName(ctx context.Context, c fleetAPI, name string) (*gltypes.FleetAttributes, error) {
	listToken := ""
	for {
		listIn := &gamelift.ListFleetsInput{}
		if listToken != "" {
			listIn.NextToken = aws.String(listToken)
		}
		listOut, err := c.ListFleets(ctx, listIn)
		if err != nil {
			return nil, fmt.Errorf("listing fleets: %w", err)
		}

		fleet, err := describePageForName(ctx, c, listOut.FleetIds, name)
		if err != nil {
			return nil, err
		}
		if fleet != nil {
			return fleet, nil
		}

		if aws.ToString(listOut.NextToken) == "" {
			return nil, fmt.Errorf("no fleet found with name %s", name)
		}
		listToken = aws.ToString(listOut.NextToken)
	}
}

// describePageForName describes one batch of fleet IDs, following the
// describe-level NextToken, and returns the matching fleet or nil.
func describePageForName(ctx context.Context, c fleetAPI, fleetIDs []string, name string) (*gltypes.FleetAttributes, error) {
	descToken := ""
	for {
		descIn := &gamelift.DescribeFleetAttributesInput{
			FleetIds: fleetIDs,
		}
		if descToken != "" {
			descIn.NextToken = aws.String(descToken)
		}

		descOut, err := c.DescribeFleetAttributes(ctx, descIn)
		if err != nil {
			return nil, fmt.Errorf("describing fleet attributes: %w", err)
		}

		for i := range descOut.FleetAttributes {
			if aws.ToString(descOut.FleetAttributes[i].Name) == name {
				return &descOut.FleetAttributes[i], nil
			}
		}

		if aws.ToString(descOut.NextToken) == "" {
			return nil, nil
		}
		descToken = aws.ToString(descOut.NextToken)
	}
}

// GetFleetStatus looks up the fleet by name and returns its current status.
func (d *Deployer) GetFleetStatus(ctx context.Context) (*FleetStatus, error) {
	fleet, err := findFleetByName(ctx, d.glClient, d.opts.FleetName)
	if err != nil {
		return nil, err
	}
	return &FleetStatus{
		FleetID: aws.ToString(fleet.FleetId),
		Status:  string(fleet.Status),
	}, nil
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
