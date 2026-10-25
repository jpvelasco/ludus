package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jpvelasco/ludus/internal/awsutil"
)

// Deleter is the narrow seam callers use to drive durable-artifact deletion
// without depending on the concrete Cleaner. *Cleaner satisfies it; callers'
// tests inject a fake.
type Deleter interface {
	DeleteECRRepository(ctx context.Context, repoName string) error
	DeleteS3Bucket(ctx context.Context, bucketName string) error
}

// ecrAPI is the subset of ECR operations needed for cleanup.
type ecrAPI interface {
	ListImages(ctx context.Context, params *ecr.ListImagesInput, optFns ...func(*ecr.Options)) (*ecr.ListImagesOutput, error)
	BatchDeleteImage(ctx context.Context, params *ecr.BatchDeleteImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error)
	DeleteRepository(ctx context.Context, params *ecr.DeleteRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.DeleteRepositoryOutput, error)
}

// s3API is the subset of S3 operations needed for cleanup.
type s3API interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
}

// Cleaner handles cleanup of AWS resources.
type Cleaner struct {
	ecr ecrAPI
	s3  s3API
}

// compile-time assertion that the concrete cleaner satisfies the Deleter seam
// callers program against.
var _ Deleter = (*Cleaner)(nil)

// NewCleaner creates a Cleaner from an AWS config.
func NewCleaner(awsCfg aws.Config) *Cleaner {
	return &Cleaner{
		ecr: ecr.NewFromConfig(awsCfg),
		s3:  s3.NewFromConfig(awsCfg),
	}
}

// DeleteECRRepository deletes an ECR repository and all its images.
// If the repository does not exist, it prints a skip message and returns nil.
func (c *Cleaner) DeleteECRRepository(ctx context.Context, repoName string) error {
	fmt.Printf("Deleting ECR repository %s...\n", repoName)

	listOutput, err := c.ecr.ListImages(ctx, &ecr.ListImagesInput{
		RepositoryName: aws.String(repoName),
	})
	if err != nil {
		return c.handleECRNotFound(err, repoName, "list images in repository %s: %w")
	}

	if err := c.batchDeleteImages(ctx, repoName, listOutput.ImageIds); err != nil {
		return err
	}

	_, err = c.ecr.DeleteRepository(ctx, &ecr.DeleteRepositoryInput{
		RepositoryName: aws.String(repoName),
		Force:          true,
	})
	if err != nil {
		return c.handleECRNotFound(err, repoName, "delete repository %s: %w")
	}

	fmt.Println("ECR repository deleted.")
	return nil
}

// handleECRNotFound returns nil with a skip message for not-found errors,
// or wraps real errors with the given format (must contain %s and %w).
func (c *Cleaner) handleECRNotFound(err error, repoName, errFmt string) error {
	if awsutil.IsNotFound(err) {
		fmt.Printf("  ECR repository %s not found, skipping\n", repoName)
		return nil
	}
	return fmt.Errorf(errFmt, repoName, err)
}

// batchDeleteImages deletes ECR images in batches of 100. A successful call
// can still report per-image failures; they surface as an error so callers
// don't proceed under the assumption the images are gone.
func (c *Cleaner) batchDeleteImages(ctx context.Context, repoName string, imageIDs []ecrtypes.ImageIdentifier) error {
	if len(imageIDs) == 0 {
		return nil
	}

	deleted := 0
	for i := 0; i < len(imageIDs); i += 100 {
		end := min(i+100, len(imageIDs))
		batch := imageIDs[i:end]
		out, err := c.ecr.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
			RepositoryName: aws.String(repoName),
			ImageIds:       batch,
		})
		if err != nil {
			return c.handleECRNotFound(err, repoName, "batch delete images in repository %s: %w")
		}
		deleted += len(batch) - len(out.Failures)
		if len(out.Failures) > 0 {
			return fmt.Errorf("batch delete images in repository %s: %d of %d failed: %s",
				repoName, len(out.Failures), len(batch), renderECRImageFailures(out.Failures))
		}
	}

	fmt.Printf("  Deleted %d images\n", deleted)
	return nil
}

// renderECRImageFailures renders the per-image failures a successful
// BatchDeleteImage response can carry.
func renderECRImageFailures(failures []ecrtypes.ImageFailure) string {
	parts := make([]string, 0, len(failures))
	for _, f := range failures {
		parts = append(parts, fmt.Sprintf("%s (%s): %s",
			imageIDLabel(f.ImageId), f.FailureCode, aws.ToString(f.FailureReason)))
	}
	return strings.Join(parts, "; ")
}

// imageIDLabel renders an ECR image identifier for an error message.
func imageIDLabel(id *ecrtypes.ImageIdentifier) string {
	if id == nil {
		return "<unknown>"
	}
	if digest := aws.ToString(id.ImageDigest); digest != "" {
		return digest
	}
	return aws.ToString(id.ImageTag)
}

// DeleteS3Bucket deletes an S3 bucket and all its objects.
// If the bucket does not exist, it prints a skip message and returns nil.
func (c *Cleaner) DeleteS3Bucket(ctx context.Context, bucketName string) error {
	fmt.Printf("Deleting S3 bucket %s...\n", bucketName)

	_, err := c.s3.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		if awsutil.IsNotFound(err) {
			fmt.Printf("  S3 bucket %s not found, skipping\n", bucketName)
			return nil
		}
		return fmt.Errorf("check bucket %s: %w", bucketName, err)
	}

	if err := c.deleteAllObjects(ctx, bucketName); err != nil {
		return err
	}

	_, err = c.s3.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("delete bucket %s: %w", bucketName, err)
	}

	fmt.Println("S3 bucket deleted.")
	return nil
}

// deleteAllObjects paginates through all objects in a bucket and deletes them.
func (c *Cleaner) deleteAllObjects(ctx context.Context, bucketName string) error {
	var totalDeleted int
	var continuationToken *string

	for {
		listOutput, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucketName),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return fmt.Errorf("list objects in bucket %s: %w", bucketName, err)
		}

		if n, err := c.deleteObjectBatch(ctx, bucketName, listOutput.Contents); err != nil {
			return err
		} else {
			totalDeleted += n
		}

		if !aws.ToBool(listOutput.IsTruncated) {
			break
		}
		continuationToken = listOutput.NextContinuationToken
	}

	if totalDeleted > 0 {
		fmt.Printf("  Deleted %d objects\n", totalDeleted)
	}
	return nil
}

// deleteObjectBatch deletes a single page of S3 objects and returns the count
// confirmed deleted. A successful call can still report per-object errors;
// they surface as an error and only the non-failed objects count as deleted.
func (c *Cleaner) deleteObjectBatch(ctx context.Context, bucketName string, contents []s3types.Object) (int, error) {
	if len(contents) == 0 {
		return 0, nil
	}

	ids := make([]s3types.ObjectIdentifier, len(contents))
	for i, obj := range contents {
		ids[i] = s3types.ObjectIdentifier{Key: obj.Key}
	}

	out, err := c.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucketName),
		Delete: &s3types.Delete{Objects: ids, Quiet: aws.Bool(true)},
	})
	if err != nil {
		return 0, fmt.Errorf("delete objects in bucket %s: %w", bucketName, err)
	}
	deleted := len(ids) - len(out.Errors)
	if len(out.Errors) > 0 {
		return deleted, fmt.Errorf("delete objects in bucket %s: %d of %d failed: %s",
			bucketName, len(out.Errors), len(ids), renderS3DeleteErrors(out.Errors))
	}
	return deleted, nil
}

// renderS3DeleteErrors renders the per-object errors a successful
// DeleteObjects response can carry.
func renderS3DeleteErrors(errs []s3types.Error) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%s (%s): %s",
			aws.ToString(e.Key), aws.ToString(e.Code), aws.ToString(e.Message)))
	}
	return strings.Join(parts, "; ")
}
