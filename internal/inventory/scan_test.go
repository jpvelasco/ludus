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
)

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
