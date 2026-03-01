package ec2fleet

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/devrecon/ludus/internal/runner"
	"github.com/devrecon/ludus/internal/tags"
	"github.com/devrecon/ludus/internal/wrapper"
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

// LoadAWSConfig loads the default AWS SDK configuration for the given region.
func LoadAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
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

// ZipAndUpload creates a zip of the server build directory (including the
// Game Server Wrapper binary) and uploads it to S3.
func (d *Deployer) ZipAndUpload(ctx context.Context, serverBuildDir string) (bucket, key string, err error) {
	bucket, err = d.resolveBucket(ctx)
	if err != nil {
		return "", "", err
	}

	key = fmt.Sprintf("ludus/%s/%s.zip", d.opts.FleetName, time.Now().UTC().Format("20060102-150405"))

	// Ensure wrapper binary
	fmt.Println("Ensuring game server wrapper binary...")
	wrapperBinary, err := wrapper.EnsureBinary(ctx, d.Runner)
	if err != nil {
		return "", "", fmt.Errorf("game server wrapper: %w", err)
	}

	// Create zip file
	fmt.Println("Creating server build zip...")
	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("ludus-ec2-build-%d.zip", time.Now().UnixNano()))
	defer os.Remove(zipPath)

	if err := createBuildZip(zipPath, serverBuildDir, wrapperBinary); err != nil {
		return "", "", fmt.Errorf("creating build zip: %w", err)
	}

	// Upload to S3
	fmt.Printf("Uploading build to s3://%s/%s...\n", bucket, key)
	zipFile, err := os.Open(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("opening zip file: %w", err)
	}
	defer zipFile.Close()

	stat, _ := zipFile.Stat()
	fmt.Printf("  Upload size: %d MB\n", stat.Size()/(1024*1024))

	_, err = d.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   zipFile,
	})
	if err != nil {
		return "", "", fmt.Errorf("uploading to S3: %w", err)
	}

	fmt.Println("Upload complete.")
	return bucket, key, nil
}

// CreateBuild creates a GameLift Build resource pointing to the S3 upload.
func (d *Deployer) CreateBuild(ctx context.Context, bucket, key string) (string, error) {
	fmt.Println("Creating GameLift build...")
	out, err := d.glClient.CreateBuild(ctx, &gamelift.CreateBuildInput{
		Name:            aws.String(fmt.Sprintf("ludus-%s", d.opts.FleetName)),
		OperatingSystem: gltypes.OperatingSystemAmazonLinux2023,
		StorageLocation: &gltypes.S3Location{
			Bucket:  aws.String(bucket),
			Key:     aws.String(key),
			RoleArn: aws.String(""), // filled below after IAM role resolution
		},
		Tags: tags.ToGameLiftTags(d.resourceTags()),
	})
	if err != nil {
		// If direct S3 location fails (IAM not yet set up), try the role-based approach
		roleARN, roleErr := d.ensureIAMRole(ctx)
		if roleErr != nil {
			return "", fmt.Errorf("creating build: %w (and role creation failed: %v)", err, roleErr)
		}

		out, err = d.glClient.CreateBuild(ctx, &gamelift.CreateBuildInput{
			Name:            aws.String(fmt.Sprintf("ludus-%s", d.opts.FleetName)),
			OperatingSystem: gltypes.OperatingSystemAmazonLinux2023,
			StorageLocation: &gltypes.S3Location{
				Bucket:  aws.String(bucket),
				Key:     aws.String(key),
				RoleArn: aws.String(roleARN),
			},
			Tags: tags.ToGameLiftTags(d.resourceTags()),
		})
		if err != nil {
			return "", fmt.Errorf("creating build with role: %w", err)
		}
	}

	buildID := aws.ToString(out.Build.BuildId)
	fmt.Printf("Build created: %s\n", buildID)

	// Poll until READY
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		desc, err := d.glClient.DescribeBuild(ctx, &gamelift.DescribeBuildInput{
			BuildId: aws.String(buildID),
		})
		if err != nil {
			return buildID, fmt.Errorf("polling build status: %w", err)
		}

		status := desc.Build.Status
		fmt.Printf("  Build status: %s\n", status)
		if status == gltypes.BuildStatusReady {
			return buildID, nil
		}
		if status == gltypes.BuildStatusFailed {
			return buildID, fmt.Errorf("build failed")
		}

		time.Sleep(pollInterval)
	}

	return buildID, fmt.Errorf("timed out waiting for build to become READY")
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

	// The wrapper binary is at the root of the zip; the server binary is under
	// the project directory structure.
	launchPath := "/local/game/amazon-gamelift-servers-game-server-wrapper"
	serverBinary := fmt.Sprintf("/local/game/%s/Binaries/Linux/%s",
		d.opts.ProjectName, d.opts.ServerTarget)
	launchParams := fmt.Sprintf("--executable %s --map %s --port %d",
		serverBinary, d.opts.ServerMap, d.opts.ServerPort)

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

		time.Sleep(pollInterval)
	}

	return result, fmt.Errorf("timed out waiting for fleet to become ACTIVE")
}

// CreateGameSession creates a game session on the EC2 fleet.
func (d *Deployer) CreateGameSession(ctx context.Context, fleetID string, maxPlayers int) (*GameSessionInfo, error) {
	out, err := d.glClient.CreateGameSession(ctx, &gamelift.CreateGameSessionInput{
		FleetId:                   aws.String(fleetID),
		MaximumPlayerSessionCount: aws.Int32(int32(maxPlayers)),
	})
	if err != nil {
		return nil, fmt.Errorf("creating game session: %w", err)
	}

	info := &GameSessionInfo{
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
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("deleting fleet: %w", err)
		}

		// Poll until the fleet is gone
		deadline := time.Now().Add(maxPollWait)
		for time.Now().Before(deadline) {
			desc, err := d.glClient.DescribeFleetAttributes(ctx, &gamelift.DescribeFleetAttributesInput{
				FleetIds: []string{fleetID},
			})
			if err != nil {
				if isNotFound(err) {
					break
				}
				return fmt.Errorf("polling fleet deletion: %w", err)
			}
			if len(desc.FleetAttributes) == 0 {
				break
			}
			fmt.Println("  Waiting for fleet deletion...")
			time.Sleep(pollInterval)
		}
		fmt.Println("Fleet deleted.")
	}

	// 2. Delete build
	if buildID != "" {
		fmt.Println("Deleting build...")
		_, err := d.glClient.DeleteBuild(ctx, &gamelift.DeleteBuildInput{
			BuildId: aws.String(buildID),
		})
		if err != nil && !isNotFound(err) {
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
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("detaching policy from role: %w", err)
	}

	_, err = d.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(iamRoleName),
	})
	if err != nil {
		if isNotFound(err) {
			fmt.Println("IAM role not found, skipping.")
			return nil
		}
		return fmt.Errorf("deleting IAM role: %w", err)
	}

	fmt.Println("IAM role deleted.")
	return nil
}

// GameSessionInfo holds connection details for a game session.
type GameSessionInfo struct {
	SessionID string
	IPAddress string
	Port      int
}

// createBuildZip creates a zip file containing the server build directory and
// the game server wrapper binary at the root.
func createBuildZip(zipPath, serverBuildDir, wrapperBinary string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// Add wrapper binary at the root of the zip
	if err := addFileToZip(w, wrapperBinary, "amazon-gamelift-servers-game-server-wrapper"); err != nil {
		return fmt.Errorf("adding wrapper to zip: %w", err)
	}

	// Add server build directory contents
	return filepath.Walk(serverBuildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(serverBuildDir, path)
		if err != nil {
			return err
		}

		// Use forward slashes in zip
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		if info.IsDir() {
			if relPath == "." {
				return nil
			}
			_, err := w.Create(relPath + "/")
			return err
		}

		return addFileToZip(w, path, relPath)
	})
}

// addFileToZip adds a single file to a zip archive.
func addFileToZip(w *zip.Writer, srcPath, zipPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	// Preserve executable permission
	if info.Mode()&0111 != 0 {
		header.SetMode(info.Mode())
	}

	dst, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	return err
}

// isNotFound returns true if the error message indicates a resource was not found.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NotFoundException") ||
		strings.Contains(msg, "NoSuchEntity") ||
		strings.Contains(msg, "NotFound")
}
