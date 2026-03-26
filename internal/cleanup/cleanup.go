package cleanup

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

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

	// List all images in the repository
	listOutput, err := c.ecr.ListImages(ctx, &ecr.ListImagesInput{
		RepositoryName: aws.String(repoName),
	})
	if err != nil {
		if isNotFound(err) {
			fmt.Printf("  ECR repository %s not found, skipping\n", repoName)
			return nil
		}
		return fmt.Errorf("list images in repository %s: %w", repoName, err)
	}

	// Delete all images if any exist
	imageCount := len(listOutput.ImageIds)
	if imageCount > 0 {
		// Batch delete supports up to 100 images at a time
		for i := 0; i < imageCount; i += 100 {
			end := min(i+100, imageCount)
			batch := listOutput.ImageIds[i:end]

			_, err = c.ecr.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
				RepositoryName: aws.String(repoName),
				ImageIds:       batch,
			})
			if err != nil {
				if isNotFound(err) {
					fmt.Printf("  ECR repository %s not found, skipping\n", repoName)
					return nil
				}
				return fmt.Errorf("batch delete images in repository %s: %w", repoName, err)
			}
		}
		fmt.Printf("  Deleted %d images\n", imageCount)
	}

	// Delete the repository
	_, err = c.ecr.DeleteRepository(ctx, &ecr.DeleteRepositoryInput{
		RepositoryName: aws.String(repoName),
		Force:          true,
	})
	if err != nil {
		if isNotFound(err) {
			fmt.Printf("  ECR repository %s not found, skipping\n", repoName)
			return nil
		}
		return fmt.Errorf("delete repository %s: %w", repoName, err)
	}

	fmt.Println("ECR repository deleted.")
	return nil
}

// DeleteS3Bucket deletes an S3 bucket and all its objects.
// If the bucket does not exist, it prints a skip message and returns nil.
func (c *Cleaner) DeleteS3Bucket(ctx context.Context, bucketName string) error {
	fmt.Printf("Deleting S3 bucket %s...\n", bucketName)

	// Check if bucket exists
	_, err := c.s3.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		if isNotFound(err) {
			fmt.Printf("  S3 bucket %s not found, skipping\n", bucketName)
			return nil
		}
		return fmt.Errorf("check bucket %s: %w", bucketName, err)
	}

	// Delete all objects in the bucket
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

		if len(listOutput.Contents) > 0 {
			// Build delete objects input
			var objectsToDelete []s3types.ObjectIdentifier
			for _, obj := range listOutput.Contents {
				objectsToDelete = append(objectsToDelete, s3types.ObjectIdentifier{
					Key: obj.Key,
				})
			}

			_, err = c.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucketName),
				Delete: &s3types.Delete{
					Objects: objectsToDelete,
					Quiet:   aws.Bool(true),
				},
			})
			if err != nil {
				return fmt.Errorf("delete objects in bucket %s: %w", bucketName, err)
			}

			totalDeleted += len(objectsToDelete)
		}

		if !aws.ToBool(listOutput.IsTruncated) {
			break
		}
		continuationToken = listOutput.NextContinuationToken
	}

	if totalDeleted > 0 {
		fmt.Printf("  Deleted %d objects\n", totalDeleted)
	}

	// Delete the bucket
	_, err = c.s3.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("delete bucket %s: %w", bucketName, err)
	}

	fmt.Println("S3 bucket deleted.")
	return nil
}

// isNotFound checks if an error indicates a resource was not found.
func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "RepositoryNotFoundException", "NoSuchBucket", "NotFound", "ResourceNotFoundException", "NoSuchEntity":
			return true
		}
	}

	// Check for HTTP 404 status code (S3 HeadBucket returns this)
	var oe *smithy.OperationError
	if errors.As(err, &oe) {
		// Check if it contains a 404 status
		errStr := oe.Error()
		if errStr == "" && oe.Unwrap() != nil {
			errStr = oe.Unwrap().Error()
		}
		if errStr != "" && (errStr == "NotFound" || errStr == "404") {
			return true
		}
	}

	return false
}
