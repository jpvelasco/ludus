package ec2fleet

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/devrecon/ludus/internal/tags"
)

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

// deleteS3Object removes the server build archive from S3.
func (d *Deployer) deleteS3Object(ctx context.Context, bucket, key string) {
	if bucket == "" || key == "" {
		return
	}

	fmt.Printf("Deleting S3 object s3://%s/%s...\n", bucket, key)
	_, err := d.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		fmt.Printf("Warning: failed to delete S3 object: %v\n", err)
		return
	}
	fmt.Println("S3 object deleted.")
}
