package inventory

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// Package-level test vars for common mock objects.
var ecrNotFoundErr = &smithy.GenericAPIError{Code: "RepositoryNotFoundException", Message: "not found"}

// Helper constructors for common mocks.
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

// mockTaggingClient implements taggingAPI for testing.
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

// mockECRClient implements ecrAPI for testing.
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

// mockS3Client implements s3API for testing.
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

var scanTests = []struct {
	name           string
	tagging        *mockTaggingClient
	ecr            *mockECRClient
	s3             *mockS3Client
	ecrRepoNames   []string
	s3BucketPrefix string
	wantCount      int
	wantErr        bool
}{
	{
		name:           "empty_account",
		tagging:        emptyTaggingClient(),
		ecr:            notFoundECRClient(),
		s3:             emptyS3Client(),
		ecrRepoNames:   []string{"my-repo"},
		s3BucketPrefix: "ludus-builds-",
		wantCount:      0,
	},
	{
		name: "tagged_resources_found",
		tagging: &mockTaggingClient{
			outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
				{
					ResourceTagMappingList: []tagtypes.ResourceTagMapping{
						{ResourceARN: aws.String("arn:aws:gamelift:us-east-1:123456789012:fleet/fleet-abc")},
						{ResourceARN: aws.String("arn:aws:iam::123456789012:role/LudusRole")},
						{ResourceARN: aws.String("arn:aws:cloudformation:us-east-1:123456789012:stack/my-stack/guid-123")},
					},
				},
			},
		},
		ecr:            notFoundECRClient(),
		s3:             emptyS3Client(),
		ecrRepoNames:   []string{"my-repo"},
		s3BucketPrefix: "",
		wantCount:      3,
	},
	{
		name:    "ecr_repos_found_by_name",
		tagging: emptyTaggingClient(),
		ecr: &mockECRClient{
			describeOutput: &ecr.DescribeRepositoriesOutput{
				Repositories: []ecrtypes.Repository{
					{
						RepositoryName: aws.String("ludus-game-server"),
						RepositoryArn:  aws.String("arn:aws:ecr:us-east-1:123:repository/ludus-game-server"),
					},
				},
			},
			listOutput: &ecr.ListImagesOutput{
				ImageIds: []ecrtypes.ImageIdentifier{
					{ImageDigest: aws.String("sha256:aaa")},
					{ImageDigest: aws.String("sha256:bbb")},
					{ImageDigest: aws.String("sha256:ccc")},
					{ImageDigest: aws.String("sha256:ddd")},
					{ImageDigest: aws.String("sha256:eee")},
				},
			},
		},
		s3:             emptyS3Client(),
		ecrRepoNames:   []string{"ludus-game-server"},
		s3BucketPrefix: "",
		wantCount:      1,
	},
	{
		name:           "ecr_repo_not_found",
		tagging:        emptyTaggingClient(),
		ecr:            notFoundECRClient(),
		s3:             emptyS3Client(),
		ecrRepoNames:   []string{"nonexistent-repo"},
		s3BucketPrefix: "",
		wantCount:      0,
	},
	{
		name:    "s3_bucket_found_by_prefix",
		tagging: emptyTaggingClient(),
		ecr:     notFoundECRClient(),
		s3: &mockS3Client{
			listOutput: &s3.ListBucketsOutput{
				Buckets: []s3types.Bucket{
					{Name: aws.String("ludus-builds-myproject")},
					{Name: aws.String("unrelated-bucket")},
				},
			},
			taggingOutputs: map[string]*s3.GetBucketTaggingOutput{
				"ludus-builds-myproject": {
					TagSet: []s3types.Tag{
						{Key: aws.String("ManagedBy"), Value: aws.String("ludus")},
					},
				},
			},
		},
		ecrRepoNames:   []string{"my-repo"},
		s3BucketPrefix: "ludus-builds-",
		wantCount:      1,
	},
	{
		name: "dedup_tagging_and_s3",
		tagging: &mockTaggingClient{
			outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
				{
					ResourceTagMappingList: []tagtypes.ResourceTagMapping{
						{ResourceARN: aws.String("arn:aws:s3:::ludus-builds-myproject")},
					},
				},
			},
		},
		ecr: notFoundECRClient(),
		s3: &mockS3Client{
			listOutput: &s3.ListBucketsOutput{
				Buckets: []s3types.Bucket{
					{Name: aws.String("ludus-builds-myproject")},
				},
			},
		},
		ecrRepoNames:   []string{"my-repo"},
		s3BucketPrefix: "ludus-builds-",
		wantCount:      1,
	},
	{
		name: "tagging_api_pagination",
		tagging: &mockTaggingClient{
			outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
				{
					ResourceTagMappingList: []tagtypes.ResourceTagMapping{
						{ResourceARN: aws.String("arn:aws:gamelift:us-east-1:123:fleet/fleet-1")},
					},
					PaginationToken: aws.String("token-page-2"),
				},
				{
					ResourceTagMappingList: []tagtypes.ResourceTagMapping{
						{ResourceARN: aws.String("arn:aws:gamelift:us-east-1:123:fleet/fleet-2")},
						{ResourceARN: aws.String("arn:aws:iam::123:role/MyRole")},
					},
				},
			},
		},
		ecr:            notFoundECRClient(),
		s3:             emptyS3Client(),
		ecrRepoNames:   []string{"my-repo"},
		s3BucketPrefix: "",
		wantCount:      3,
	},
}

func TestScan(t *testing.T) {
	for _, tt := range scanTests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScanner(tt.tagging, tt.ecr, tt.s3, tt.ecrRepoNames, tt.s3BucketPrefix)

			inv, err := s.Scan(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Scan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(inv.Resources) != tt.wantCount {
				t.Errorf("Scan() returned %d resources, want %d", len(inv.Resources), tt.wantCount)
				for i, r := range inv.Resources {
					t.Logf("  resource[%d]: type=%s name=%s arn=%s", i, r.Type, r.Name, r.ARN)
				}
			}
		})
	}
}

func TestScanResourceTypes(t *testing.T) {
	// Verify that tagged resources are categorized correctly
	tagging := &mockTaggingClient{
		outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
			{
				ResourceTagMappingList: []tagtypes.ResourceTagMapping{
					{ResourceARN: aws.String("arn:aws:gamelift:us-east-1:123:fleet/fleet-abc")},
					{ResourceARN: aws.String("arn:aws:iam::123:role/LudusRole")},
					{ResourceARN: aws.String("arn:aws:cloudformation:us-east-1:123:stack/my-stack/guid")},
				},
			},
		},
	}

	s := newTestScanner(tagging, notFoundECRClient(), emptyS3Client(), nil, "")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	wantTypes := map[string]string{
		"fleet-abc": "GameLift Fleet",
		"LudusRole": "IAM Role",
		"my-stack":  "CloudFormation Stack",
	}

	for _, r := range inv.Resources {
		wantType, ok := wantTypes[r.Name]
		if !ok {
			t.Errorf("unexpected resource name %q", r.Name)
			continue
		}
		if r.Type != wantType {
			t.Errorf("resource %q: type = %q, want %q", r.Name, r.Type, wantType)
		}
	}
}

func TestScanECRDetail(t *testing.T) {
	ecrClient := &mockECRClient{
		describeOutput: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{
				{
					RepositoryName: aws.String("ludus-server"),
					RepositoryArn:  aws.String("arn:aws:ecr:us-east-1:123:repository/ludus-server"),
				},
			},
		},
		listOutput: &ecr.ListImagesOutput{
			ImageIds: []ecrtypes.ImageIdentifier{
				{ImageDigest: aws.String("sha256:aaa")},
				{ImageDigest: aws.String("sha256:bbb")},
				{ImageDigest: aws.String("sha256:ccc")},
				{ImageDigest: aws.String("sha256:ddd")},
				{ImageDigest: aws.String("sha256:eee")},
			},
		},
	}
	s := newTestScanner(emptyTaggingClient(), ecrClient, emptyS3Client(), []string{"ludus-server"}, "")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(inv.Resources))
	}
	if inv.Resources[0].Detail != "5 images" {
		t.Errorf("detail = %q, want %q", inv.Resources[0].Detail, "5 images")
	}
}

func TestHasTag(t *testing.T) {
	tests := []struct {
		name  string
		tags  []s3types.Tag
		key   string
		value string
		want  bool
	}{
		{
			name: "matching tag",
			tags: []s3types.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("ludus")}},
			key:  "ManagedBy", value: "ludus", want: true,
		},
		{
			name: "wrong value",
			tags: []s3types.Tag{{Key: aws.String("ManagedBy"), Value: aws.String("other")}},
			key:  "ManagedBy", value: "ludus", want: false,
		},
		{
			name: "wrong key",
			tags: []s3types.Tag{{Key: aws.String("Owner"), Value: aws.String("ludus")}},
			key:  "ManagedBy", value: "ludus", want: false,
		},
		{
			name: "empty tags",
			tags: nil,
			key:  "ManagedBy", value: "ludus", want: false,
		},
		{
			name: "multiple tags finds match",
			tags: []s3types.Tag{
				{Key: aws.String("Env"), Value: aws.String("prod")},
				{Key: aws.String("ManagedBy"), Value: aws.String("ludus")},
			},
			key: "ManagedBy", value: "ludus", want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTag(tt.tags, tt.key, tt.value)
			if got != tt.want {
				t.Errorf("hasTag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseARN(t *testing.T) {
	tests := []struct {
		name        string
		arn         string
		wantService string
		wantType    string
		wantName    string
	}{
		{
			name:        "gamelift_fleet",
			arn:         "arn:aws:gamelift:us-east-1:123456789012:fleet/fleet-abc",
			wantService: "gamelift",
			wantType:    "fleet",
			wantName:    "fleet-abc",
		},
		{
			name:        "iam_role",
			arn:         "arn:aws:iam::123456789012:role/LudusRole",
			wantService: "iam",
			wantType:    "role",
			wantName:    "LudusRole",
		},
		{
			name:        "cloudformation_stack",
			arn:         "arn:aws:cloudformation:us-east-1:123456789012:stack/my-stack/guid-123",
			wantService: "cloudformation",
			wantType:    "stack",
			wantName:    "my-stack",
		},
		{
			name:        "s3_bucket",
			arn:         "arn:aws:s3:::my-bucket",
			wantService: "s3",
			wantType:    "",
			wantName:    "my-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, resourceType, resourceName := parseARN(tt.arn)
			if service != tt.wantService {
				t.Errorf("parseARN(%q) service = %q, want %q", tt.arn, service, tt.wantService)
			}
			if resourceType != tt.wantType {
				t.Errorf("parseARN(%q) type = %q, want %q", tt.arn, resourceType, tt.wantType)
			}
			if resourceName != tt.wantName {
				t.Errorf("parseARN(%q) name = %q, want %q", tt.arn, resourceName, tt.wantName)
			}
		})
	}
}

func TestHumanizeResourceType(t *testing.T) {
	tests := []struct {
		name         string
		service      string
		resourceType string
		want         string
	}{
		{
			name:         "gamelift_fleet",
			service:      "gamelift",
			resourceType: "fleet",
			want:         "GameLift Fleet",
		},
		{
			name:         "iam_role",
			service:      "iam",
			resourceType: "role",
			want:         "IAM Role",
		},
		{
			name:         "unknown_service",
			service:      "unknown",
			resourceType: "thing",
			want:         "Unknown Thing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanizeResourceType(tt.service, tt.resourceType)
			if got != tt.want {
				t.Errorf("humanizeResourceType(%q, %q) = %q, want %q", tt.service, tt.resourceType, got, tt.want)
			}
		})
	}
}
