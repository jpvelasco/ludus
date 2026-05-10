package cleanup

import (
	"context"
	"errors"
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
