package cleanup

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"testing"
)

// mockECRClient implements ecrAPI for testing.
type mockECRClient struct {
	listImagesOutput    *ecr.ListImagesOutput
	listImagesErr       error
	batchDeleteImageErr error
	deleteRepositoryErr error
}

func (m *mockECRClient) ListImages(ctx context.Context, params *ecr.ListImagesInput, optFns ...func(*ecr.Options)) (*ecr.ListImagesOutput, error) {
	if m.listImagesErr != nil {
		return nil, m.listImagesErr
	}
	return m.listImagesOutput, nil
}

func (m *mockECRClient) BatchDeleteImage(ctx context.Context, params *ecr.BatchDeleteImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error) {
	if m.batchDeleteImageErr != nil {
		return nil, m.batchDeleteImageErr
	}
	return &ecr.BatchDeleteImageOutput{}, nil
}

func (m *mockECRClient) DeleteRepository(ctx context.Context, params *ecr.DeleteRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.DeleteRepositoryOutput, error) {
	if m.deleteRepositoryErr != nil {
		return nil, m.deleteRepositoryErr
	}
	return &ecr.DeleteRepositoryOutput{}, nil
}

// mockS3Client implements s3API for testing.
type mockS3Client struct {
	headBucketErr      error
	listObjectsOutputs []*s3.ListObjectsV2Output
	listObjectsCallIdx int
	deleteObjectsErr   error
	deleteBucketErr    error
}

func (m *mockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if m.headBucketErr != nil {
		return nil, m.headBucketErr
	}
	return &s3.HeadBucketOutput{}, nil
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listObjectsCallIdx >= len(m.listObjectsOutputs) {
		return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
	}
	output := m.listObjectsOutputs[m.listObjectsCallIdx]
	m.listObjectsCallIdx++
	return output, nil
}

func (m *mockS3Client) DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if m.deleteObjectsErr != nil {
		return nil, m.deleteObjectsErr
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func (m *mockS3Client) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	if m.deleteBucketErr != nil {
		return nil, m.deleteBucketErr
	}
	return &s3.DeleteBucketOutput{}, nil
}

var deleteECRRepositoryTests = []struct {
	name    string
	mock    *mockECRClient
	wantErr bool
}{
	{
		name: "repo_with_images",
		mock: &mockECRClient{
			listImagesOutput: &ecr.ListImagesOutput{
				ImageIds: []ecrtypes.ImageIdentifier{
					{ImageDigest: aws.String("sha256:abc123")},
					{ImageDigest: aws.String("sha256:def456")},
					{ImageDigest: aws.String("sha256:ghi789")},
				},
			},
		},
		wantErr: false,
	},
	{
		name: "repo_not_found",
		mock: &mockECRClient{
			listImagesErr: &smithy.GenericAPIError{
				Code:    "RepositoryNotFoundException",
				Message: "not found",
			},
		},
		wantErr: false,
	},
	{
		name: "api_error",
		mock: &mockECRClient{
			listImagesErr: errors.New("generic AWS error"),
		},
		wantErr: true,
	},
	{
		name: "repo_with_no_images",
		mock: &mockECRClient{
			listImagesOutput: &ecr.ListImagesOutput{
				ImageIds: []ecrtypes.ImageIdentifier{},
			},
		},
		wantErr: false,
	},
	{
		name: "batch_delete_error",
		mock: &mockECRClient{
			listImagesOutput: &ecr.ListImagesOutput{
				ImageIds: []ecrtypes.ImageIdentifier{
					{ImageDigest: aws.String("sha256:abc123")},
				},
			},
			batchDeleteImageErr: errors.New("batch delete failed"),
		},
		wantErr: true,
	},
	{
		name: "delete_repository_error",
		mock: &mockECRClient{
			listImagesOutput: &ecr.ListImagesOutput{
				ImageIds: []ecrtypes.ImageIdentifier{},
			},
			deleteRepositoryErr: errors.New("delete repository failed"),
		},
		wantErr: true,
	},
}

func TestDeleteECRRepository(t *testing.T) {
	for _, tt := range deleteECRRepositoryTests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Cleaner{
				ecr: tt.mock,
			}

			err := c.DeleteECRRepository(context.Background(), "test-repo")
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteECRRepository() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

var deleteS3BucketTests = []struct {
	name    string
	mock    *mockS3Client
	wantErr bool
}{
	{
		name: "bucket_with_objects",
		mock: &mockS3Client{
			listObjectsOutputs: []*s3.ListObjectsV2Output{
				{
					Contents: []s3types.Object{
						{Key: aws.String("file1.txt")},
						{Key: aws.String("file2.txt")},
					},
					IsTruncated: aws.Bool(false),
				},
			},
		},
		wantErr: false,
	},
	{
		name: "bucket_not_found",
		mock: &mockS3Client{
			headBucketErr: &smithy.GenericAPIError{
				Code:    "NotFound",
				Message: "bucket not found",
			},
		},
		wantErr: false,
	},
	{
		name: "bucket_paginated",
		mock: &mockS3Client{
			listObjectsOutputs: []*s3.ListObjectsV2Output{
				{
					Contents: []s3types.Object{
						{Key: aws.String("file1.txt")},
					},
					IsTruncated:           aws.Bool(true),
					NextContinuationToken: aws.String("token123"),
				},
				{
					Contents: []s3types.Object{
						{Key: aws.String("file2.txt")},
					},
					IsTruncated: aws.Bool(false),
				},
			},
		},
		wantErr: false,
	},
	{
		name: "bucket_empty",
		mock: &mockS3Client{
			listObjectsOutputs: []*s3.ListObjectsV2Output{
				{
					Contents:    []s3types.Object{},
					IsTruncated: aws.Bool(false),
				},
			},
		},
		wantErr: false,
	},
	{
		name: "head_bucket_error",
		mock: &mockS3Client{
			headBucketErr: errors.New("head bucket failed"),
		},
		wantErr: true,
	},
	{
		name: "delete_objects_error",
		mock: &mockS3Client{
			listObjectsOutputs: []*s3.ListObjectsV2Output{
				{
					Contents: []s3types.Object{
						{Key: aws.String("file1.txt")},
					},
					IsTruncated: aws.Bool(false),
				},
			},
			deleteObjectsErr: errors.New("delete objects failed"),
		},
		wantErr: true,
	},
	{
		name: "delete_bucket_error",
		mock: &mockS3Client{
			listObjectsOutputs: []*s3.ListObjectsV2Output{
				{
					Contents:    []s3types.Object{},
					IsTruncated: aws.Bool(false),
				},
			},
			deleteBucketErr: errors.New("delete bucket failed"),
		},
		wantErr: true,
	},
}

func TestDeleteS3Bucket(t *testing.T) {
	for _, tt := range deleteS3BucketTests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Cleaner{
				s3: tt.mock,
			}

			err := c.DeleteS3Bucket(context.Background(), "test-bucket")
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteS3Bucket() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
