package cleanup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/smithy-go"
)

// mockECRClient implements ecrAPI for testing.
type mockECRClient struct {
	listImagesOutput    *ecr.ListImagesOutput
	listImagesErr       error
	batchDeleteOut      *ecr.BatchDeleteImageOutput
	batchDeleteImageErr error
	deleteRepositoryErr error
	deleteRepositoryCnt int
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
	if m.batchDeleteOut != nil {
		return m.batchDeleteOut, nil
	}
	return &ecr.BatchDeleteImageOutput{}, nil
}

func (m *mockECRClient) DeleteRepository(ctx context.Context, params *ecr.DeleteRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.DeleteRepositoryOutput, error) {
	m.deleteRepositoryCnt++
	if m.deleteRepositoryErr != nil {
		return nil, m.deleteRepositoryErr
	}
	return &ecr.DeleteRepositoryOutput{}, nil
}

func TestNewCleaner(t *testing.T) {
	// Constructing a Cleaner from a zero config only builds clients — no AWS
	// calls happen until a method is invoked.
	c := NewCleaner(aws.Config{})
	if c == nil {
		t.Fatal("NewCleaner returned nil")
	}
	if c.ecr == nil || c.s3 == nil {
		t.Error("NewCleaner left ecr/s3 clients nil")
	}
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
	{
		// A successful BatchDeleteImage response can still carry per-image
		// failures; they must surface instead of reporting the batch deleted.
		name: "partial_image_delete_failures",
		mock: &mockECRClient{
			listImagesOutput: &ecr.ListImagesOutput{
				ImageIds: []ecrtypes.ImageIdentifier{
					{ImageDigest: aws.String("sha256:a")},
					{ImageDigest: aws.String("sha256:b")},
					{ImageDigest: aws.String("sha256:c")},
				},
			},
			batchDeleteOut: &ecr.BatchDeleteImageOutput{
				ImageIds: []ecrtypes.ImageIdentifier{
					{ImageDigest: aws.String("sha256:a")},
					{ImageDigest: aws.String("sha256:c")},
				},
				Failures: []ecrtypes.ImageFailure{
					{
						ImageId:       &ecrtypes.ImageIdentifier{ImageDigest: aws.String("sha256:b")},
						FailureCode:   "ImageNotFound",
						FailureReason: aws.String("image referenced by another manifest"),
					},
				},
			},
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
			if tt.name == "partial_image_delete_failures" && tt.mock.deleteRepositoryCnt != 0 {
				t.Errorf("DeleteRepository calls = %d, want 0 when image deletion failed", tt.mock.deleteRepositoryCnt)
			}
		})
	}
}

// TestBatchDeleteImagesReportsPartialFailures is the regression test for
// per-image ECR delete failures hidden inside a successful HTTP response: the
// batch must return a contextual error naming the failed image.
func TestBatchDeleteImagesReportsPartialFailures(t *testing.T) {
	imageIDs := []ecrtypes.ImageIdentifier{
		{ImageDigest: aws.String("sha256:a")},
		{ImageDigest: aws.String("sha256:b")},
		{ImageDigest: aws.String("sha256:c")},
	}
	m := &mockECRClient{batchDeleteOut: &ecr.BatchDeleteImageOutput{
		ImageIds: []ecrtypes.ImageIdentifier{
			{ImageDigest: aws.String("sha256:a")},
			{ImageDigest: aws.String("sha256:c")},
		},
		Failures: []ecrtypes.ImageFailure{
			{
				ImageId:       &ecrtypes.ImageIdentifier{ImageDigest: aws.String("sha256:b")},
				FailureCode:   "ImageNotFound",
				FailureReason: aws.String("referenced by another manifest"),
			},
		},
	}}
	c := &Cleaner{ecr: m}

	err := c.batchDeleteImages(context.Background(), "test-repo", imageIDs)
	if err == nil {
		t.Fatal("batchDeleteImages() error = nil, want partial failure")
	}
	for _, want := range []string{"1 of 3", "sha256:b", "ImageNotFound"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("batchDeleteImages() error = %v, want it to mention %q", err, want)
		}
	}
}

// TestImageIDLabel pins how failed image identifiers render in errors.
func TestImageIDLabel(t *testing.T) {
	tests := []struct {
		name string
		id   *ecrtypes.ImageIdentifier
		want string
	}{
		{"digest wins", &ecrtypes.ImageIdentifier{ImageDigest: aws.String("sha256:a"), ImageTag: aws.String("v1")}, "sha256:a"},
		{"tag used when no digest", &ecrtypes.ImageIdentifier{ImageTag: aws.String("v1")}, "v1"},
		{"nil identifier is unknown", nil, "<unknown>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := imageIDLabel(tt.id); got != tt.want {
				t.Errorf("imageIDLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBatchDeleteImagesCountsFullSuccess pins the happy path: without item
// failures the requested images are reported deleted.
func TestBatchDeleteImagesCountsFullSuccess(t *testing.T) {
	imageIDs := []ecrtypes.ImageIdentifier{
		{ImageDigest: aws.String("sha256:a")},
		{ImageDigest: aws.String("sha256:b")},
	}
	c := &Cleaner{ecr: &mockECRClient{}}

	if err := c.batchDeleteImages(context.Background(), "test-repo", imageIDs); err != nil {
		t.Fatalf("batchDeleteImages() error = %v", err)
	}
}
