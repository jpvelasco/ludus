package cleanup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// mockS3Client implements s3API for testing.
type mockS3Client struct {
	headBucketErr      error
	listObjectsOutputs []*s3.ListObjectsV2Output
	listObjectsCallIdx int
	deleteObjectsOut   *s3.DeleteObjectsOutput
	deleteObjectsErr   error
	deleteBucketErr    error
	deleteBucketCalls  int
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
	if m.deleteObjectsOut != nil {
		return m.deleteObjectsOut, nil
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func (m *mockS3Client) DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	m.deleteBucketCalls++
	if m.deleteBucketErr != nil {
		return nil, m.deleteBucketErr
	}
	return &s3.DeleteBucketOutput{}, nil
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
	{
		// A 200 response can still carry per-object errors; they must surface
		// as a failure instead of counting the whole batch as deleted.
		name: "partial_object_delete_failures",
		mock: &mockS3Client{
			listObjectsOutputs: []*s3.ListObjectsV2Output{
				{
					Contents: []s3types.Object{
						{Key: aws.String("file1.txt")},
						{Key: aws.String("file2.txt")},
						{Key: aws.String("file3.txt")},
					},
					IsTruncated: aws.Bool(false),
				},
			},
			deleteObjectsOut: &s3.DeleteObjectsOutput{
				Errors: []s3types.Error{
					{Key: aws.String("file2.txt"), Code: aws.String("AccessDenied"), Message: aws.String("denied")},
				},
			},
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
			if tt.name == "partial_object_delete_failures" && tt.mock.deleteBucketCalls != 0 {
				t.Errorf("DeleteBucket calls = %d, want 0 when objects failed to delete", tt.mock.deleteBucketCalls)
			}
		})
	}
}

// TestDeleteObjectBatchReportsPartialFailures is the regression test for
// per-object S3 delete errors hidden inside a successful HTTP response: the
// batch must return a contextual error and count only confirmed deletions.
func TestDeleteObjectBatchReportsPartialFailures(t *testing.T) {
	contents := []s3types.Object{
		{Key: aws.String("file1.txt")},
		{Key: aws.String("file2.txt")},
		{Key: aws.String("file3.txt")},
	}
	m := &mockS3Client{deleteObjectsOut: &s3.DeleteObjectsOutput{
		Errors: []s3types.Error{
			{Key: aws.String("file2.txt"), Code: aws.String("AccessDenied"), Message: aws.String("permission denied")},
		},
	}}
	c := &Cleaner{s3: m}

	deleted, err := c.deleteObjectBatch(context.Background(), "test-bucket", contents)
	if err == nil {
		t.Fatal("deleteObjectBatch() error = nil, want partial failure")
	}
	for _, want := range []string{"1 of 3", "file2.txt", "AccessDenied"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("deleteObjectBatch() error = %v, want it to mention %q", err, want)
		}
	}
	if deleted != 2 {
		t.Errorf("deleteObjectBatch() deleted = %d, want 2 confirmed deletions", deleted)
	}
}

// TestDeleteObjectBatchCountsFullSuccess pins the happy path: without item
// errors every requested object counts as deleted.
func TestDeleteObjectBatchCountsFullSuccess(t *testing.T) {
	contents := []s3types.Object{{Key: aws.String("a")}, {Key: aws.String("b")}}
	c := &Cleaner{s3: &mockS3Client{}}

	deleted, err := c.deleteObjectBatch(context.Background(), "test-bucket", contents)
	if err != nil {
		t.Fatalf("deleteObjectBatch() error = %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleteObjectBatch() deleted = %d, want 2", deleted)
	}
}
