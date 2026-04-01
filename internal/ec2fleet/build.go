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
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/tags"
	"github.com/devrecon/ludus/internal/wrapper"
)

// ZipAndUpload creates a zip of the server build directory (including the
// Game Server Wrapper binary) and uploads it to S3.
func (d *Deployer) ZipAndUpload(ctx context.Context, serverBuildDir string) (bucket, key string, err error) {
	bucket, err = d.resolveBucket(ctx)
	if err != nil {
		return "", "", err
	}

	key = fmt.Sprintf("ludus/%s/%s.zip", d.opts.FleetName, time.Now().UTC().Format("20060102-150405"))

	arch := config.NormalizeArch(d.opts.Arch)
	fmt.Printf("Ensuring game server wrapper binary (%s)...\n", arch)
	wrapperBinary, err := wrapper.EnsureBinary(ctx, d.Runner, "linux", arch)
	if err != nil {
		return "", "", fmt.Errorf("game server wrapper: %w", err)
	}

	serverBinaryPath := d.resolveServerBinaryPath(serverBuildDir, arch)
	wrapperConfig := generateEC2WrapperConfig(serverBinaryPath, d.opts.ServerMap, d.opts.ServerPort)

	fmt.Println("Creating server build zip...")
	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("ludus-ec2-build-%d.zip", time.Now().UnixNano()))
	defer os.Remove(zipPath)

	if err := createBuildZip(zipPath, serverBuildDir, wrapperBinary, wrapperConfig); err != nil {
		return "", "", fmt.Errorf("creating build zip: %w", err)
	}

	if err := d.uploadToS3(ctx, bucket, key, zipPath); err != nil {
		return "", "", err
	}
	return bucket, key, nil
}

// resolveServerBinaryPath detects the actual server binary name within the build
// directory. Development builds use the bare target name (e.g. "LyraServer"),
// while Shipping/Test builds use "<Target>-<Platform>-<Config>".
func (d *Deployer) resolveServerBinaryPath(serverBuildDir, arch string) string {
	binPlatform := config.BinariesPlatformDir(arch)
	serverBinaryName := d.opts.ServerTarget
	binDir := filepath.Join(serverBuildDir, d.opts.ProjectName, "Binaries", binPlatform)
	if entries, err := os.ReadDir(binDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, d.opts.ServerTarget+"-"+binPlatform+"-") && !strings.Contains(name, ".") {
				serverBinaryName = name
				break
			}
		}
	}
	return fmt.Sprintf("./%s/Binaries/%s/%s", d.opts.ProjectName, binPlatform, serverBinaryName)
}

func (d *Deployer) uploadToS3(ctx context.Context, bucket, key, zipPath string) error {
	fmt.Printf("Uploading build to s3://%s/%s...\n", bucket, key)
	zipFile, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip file: %w", err)
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
		return fmt.Errorf("uploading to S3: %w", err)
	}

	fmt.Println("Upload complete.")
	return nil
}

// CreateBuild creates a GameLift Build resource pointing to the S3 upload.
func (d *Deployer) CreateBuild(ctx context.Context, bucket, key string) (string, error) {
	fmt.Println("Creating GameLift build...")
	out, err := d.createBuildResource(ctx, bucket, key)
	if err != nil {
		return "", err
	}

	buildID := aws.ToString(out.Build.BuildId)
	fmt.Printf("Build created: %s\n", buildID)
	return d.pollBuildReady(ctx, buildID)
}

func (d *Deployer) createBuildResource(ctx context.Context, bucket, key string) (*gamelift.CreateBuildOutput, error) {
	input := d.buildInput(bucket, key, "")
	out, err := d.glClient.CreateBuild(ctx, input)
	if err == nil {
		return out, nil
	}

	roleARN, roleErr := d.ensureIAMRole(ctx)
	if roleErr != nil {
		return nil, fmt.Errorf("creating build: %w (and role creation failed: %v)", err, roleErr)
	}

	out, err = d.glClient.CreateBuild(ctx, d.buildInput(bucket, key, roleARN))
	if err != nil {
		return nil, fmt.Errorf("creating build with role: %w", err)
	}
	return out, nil
}

func (d *Deployer) buildInput(bucket, key, roleARN string) *gamelift.CreateBuildInput {
	return &gamelift.CreateBuildInput{
		Name:             aws.String(fmt.Sprintf("ludus-%s", d.opts.FleetName)),
		OperatingSystem:  gltypes.OperatingSystemAmazonLinux2023,
		ServerSdkVersion: aws.String("5.4.0"),
		StorageLocation: &gltypes.S3Location{
			Bucket:  aws.String(bucket),
			Key:     aws.String(key),
			RoleArn: aws.String(roleARN),
		},
		Tags: tags.ToGameLiftTags(d.resourceTags()),
	}
}

func (d *Deployer) pollBuildReady(ctx context.Context, buildID string) (string, error) {
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

		select {
		case <-ctx.Done():
			return buildID, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return buildID, fmt.Errorf("timed out waiting for build to become READY")
}

// generateEC2WrapperConfig creates the game server wrapper config.yaml content
// for a GameLift Managed EC2 deployment.
func generateEC2WrapperConfig(serverBinaryPath, serverMap string, serverPort int) string {
	return fmt.Sprintf(`# Generated by ludus for GameLift Managed EC2
log-config:
  wrapper-log-level: debug
  game-server-logs-dir: ./game-server-logs

ports:
  gamePort: %d

game-server-details:
  executable-file-path: %s
  game-server-args:
    - arg: "-port"
      val: "{{.GamePort}}"
      pos: 0
    - arg: "-Map=%s"
      pos: 1
`, serverPort, serverBinaryPath, serverMap)
}

// createBuildZip creates a zip file containing the server build directory,
// the game server wrapper binary, and its config.yaml at the root.
func createBuildZip(zipPath, serverBuildDir, wrapperBinary, wrapperConfig string) error {
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

	// Add wrapper config.yaml at the root
	configWriter, err := w.Create("config.yaml")
	if err != nil {
		return fmt.Errorf("creating config.yaml in zip: %w", err)
	}
	if _, err := configWriter.Write([]byte(wrapperConfig)); err != nil {
		return fmt.Errorf("writing config.yaml to zip: %w", err)
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

	// On Windows, files lack Unix execute bits. Force 0755 for binaries
	// (no extension = Linux binary, .sh = shell script) so they're
	// executable on the GameLift Linux instance.
	ext := filepath.Ext(zipPath)
	if ext == "" || ext == ".sh" {
		header.SetMode(0755)
	} else if info.Mode()&0111 != 0 {
		header.SetMode(info.Mode())
	}

	dst, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	return err
}
