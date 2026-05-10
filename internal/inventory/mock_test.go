package inventory

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var ecrNotFoundErr = &smithy.GenericAPIError{Code: "RepositoryNotFoundException", Message: "not found"}

func emptyTaggingClient() *mockTaggingClient {
	return &mockTaggingClient{
		outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
			{ResourceTagMappingList: []tagtypes.ResourceTagMapping{}},
		},
	}
}

func notFoundECRClient() *mockECRClient {
	return &mockECRClient{describeErr: ecrNotFoundErr}
}

func emptyS3Client() *mockS3Client {
	return &mockS3Client{listOutput: &s3.ListBucketsOutput{}}
}

func newTestScanner(tagging *mockTaggingClient, ecr *mockECRClient, s3c *mockS3Client, ecrRepos []string, s3Prefix string) *Scanner {
	return &Scanner{
		tagging:        tagging,
		ecr:            ecr,
		s3:             s3c,
		region:         "us-east-1",
		ecrRepoNames:   ecrRepos,
		s3BucketPrefix: s3Prefix,
	}
}

type mockTaggingClient struct {
	outputs []*resourcegroupstaggingapi.GetResourcesOutput
	callIdx int
	err     error
}

func (m *mockTaggingClient) GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.callIdx >= len(m.outputs) {
		return &resourcegroupstaggingapi.GetResourcesOutput{}, nil
	}
	output := m.outputs[m.callIdx]
	m.callIdx++
	return output, nil
}

type mockECRClient struct {
	describeOutput *ecr.DescribeRepositoriesOutput
	describeErr    error
	listOutput     *ecr.ListImagesOutput
	listErr        error
}

func (m *mockECRClient) DescribeRepositories(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	return m.describeOutput, nil
}

func (m *mockECRClient) ListImages(ctx context.Context, params *ecr.ListImagesInput, optFns ...func(*ecr.Options)) (*ecr.ListImagesOutput, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listOutput, nil
}

type mockS3Client struct {
	listOutput     *s3.ListBucketsOutput
	listErr        error
	taggingOutputs map[string]*s3.GetBucketTaggingOutput
	taggingErr     error
}

func (m *mockS3Client) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listOutput, nil
}

func (m *mockS3Client) GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	if m.taggingErr != nil {
		return nil, m.taggingErr
	}
	if m.taggingOutputs != nil {
		if output, ok := m.taggingOutputs[aws.ToString(params.Bucket)]; ok {
			return output, nil
		}
	}
	return &s3.GetBucketTaggingOutput{}, nil
}
