package inventory

import (
	"context"
	"strings"
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

var ecrInternalErr = &smithy.GenericAPIError{Code: "InternalError", Message: "boom"}

func TestScanTaggingError(t *testing.T) {
	tagging := &mockTaggingClient{err: ecrInternalErr}
	s := newTestScanner(tagging, notFoundECRClient(), emptyS3Client(), nil, "")

	_, err := s.Scan(context.Background())
	if err == nil {
		t.Fatal("Scan() expected error from tagging API, got nil")
	}
}

func TestScanECRDescribeErrorWarns(t *testing.T) {
	ecrClient := &mockECRClient{describeErr: ecrInternalErr}
	s := newTestScanner(emptyTaggingClient(), ecrClient, emptyS3Client(), []string{"my-repo"}, "")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v, want nil (warning path)", err)
	}
	if len(inv.Resources) != 0 {
		t.Errorf("Scan() returned %d resources, want 0 (describe error skipped repo)", len(inv.Resources))
	}
	if len(inv.Warnings) != 1 || !strings.Contains(inv.Warnings[0], "my-repo") {
		t.Errorf("Scan() warnings = %v, want exactly one mentioning my-repo", inv.Warnings)
	}
}

func TestScanECRNoARN(t *testing.T) {
	ecrClient := &mockECRClient{
		describeOutput: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{
				{RepositoryName: aws.String("ludus-server"),
					RepositoryArn: aws.String("")},
			},
		},
		listOutput: &ecr.ListImagesOutput{ImageIds: []ecrtypes.ImageIdentifier{}},
	}
	s := newTestScanner(emptyTaggingClient(), ecrClient, emptyS3Client(), []string{"ludus-server"}, "")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(inv.Resources))
	}
	if inv.Resources[0].Type != "ECR Repository" {
		t.Errorf("type = %q, want ECR Repository", inv.Resources[0].Type)
	}
}

func TestScanECRImageListError(t *testing.T) {
	ecrClient := &mockECRClient{
		describeOutput: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{
				{RepositoryName: aws.String("ludus-server"),
					RepositoryArn: aws.String("arn:aws:ecr:us-east-1:123:repository/ludus-server")},
			},
		},
		listErr: ecrInternalErr,
	}
	s := newTestScanner(emptyTaggingClient(), ecrClient, emptyS3Client(), []string{"ludus-server"}, "")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(inv.Resources))
	}
	if inv.Resources[0].Detail != "" {
		t.Errorf("detail = %q, want empty on list error", inv.Resources[0].Detail)
	}
	if len(inv.Warnings) != 1 {
		t.Errorf("Scan() warnings = %v, want exactly one for the image-list failure", inv.Warnings)
	}
}

func TestScanS3ListErrorWarns(t *testing.T) {
	s3Client := &mockS3Client{listErr: ecrInternalErr}
	s := newTestScanner(emptyTaggingClient(), notFoundECRClient(), s3Client, nil, "ludus-builds-")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v, want nil (warning path)", err)
	}
	if len(inv.Resources) != 0 {
		t.Errorf("Scan() returned %d resources, want 0", len(inv.Resources))
	}
	if len(inv.Warnings) != 1 {
		t.Errorf("Scan() warnings = %v, want exactly one for the S3 list failure", inv.Warnings)
	}
}

func TestScanS3TaggingError(t *testing.T) {
	s3Client := &mockS3Client{
		listOutput: &s3.ListBucketsOutput{
			Buckets: []s3types.Bucket{{Name: aws.String("ludus-builds-x")}},
		},
		taggingErr: ecrInternalErr,
	}
	s := newTestScanner(emptyTaggingClient(), notFoundECRClient(), s3Client, nil, "ludus-builds-")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(inv.Resources))
	}
	if inv.Resources[0].Detail != "" {
		t.Errorf("detail = %q, want empty when tagging failed", inv.Resources[0].Detail)
	}
}

func TestParseARNShortForm(t *testing.T) {
	service, resourceType, resourceName := parseARN("foo")
	if service != "" || resourceType != "" || resourceName != "foo" {
		t.Errorf("parseARN(short) = (%q, %q, %q), want (\"\", \"\", \"foo\")", service, resourceType, resourceName)
	}
}

func TestParseARNColonSeparator(t *testing.T) {
	// 7 colon-separated parts: resource type + name joined by ":" (name carries guid)
	service, resourceType, resourceName := parseARN("arn:aws:cloudformation:us-east-1:123:stack:my-stack/guid")
	if service != "cloudformation" || resourceType != "stack" || resourceName != "my-stack" {
		t.Errorf("parseARN(colon) = (%q, %q, %q), want (cloudformation, stack, my-stack)", service, resourceType, resourceName)
	}
}

func TestScanECRSkipSeenARN(t *testing.T) {
	arn := "arn:aws:ecr:us-east-1:123:repository/dup-repo"
	// Tagging already found the repo ARN; the ECR name scan must skip it.
	tagging := &mockTaggingClient{
		outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
			{
				ResourceTagMappingList: []tagtypes.ResourceTagMapping{
					{ResourceARN: aws.String(arn)},
				},
			},
		},
	}
	ecrClient := &mockECRClient{
		describeOutput: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{
				{RepositoryName: aws.String("dup-repo"), RepositoryArn: aws.String(arn)},
			},
		},
		listOutput: &ecr.ListImagesOutput{},
	}
	s := newTestScanner(tagging, ecrClient, emptyS3Client(), []string{"dup-repo"}, "")
	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Errorf("Scan() returned %d resources, want 1 (seen ARN skipped)", len(inv.Resources))
	}
}

func TestScanECRSkipSeenRepo(t *testing.T) {
	ecrClient := &mockECRClient{
		describeOutput: &ecr.DescribeRepositoriesOutput{
			Repositories: []ecrtypes.Repository{
				{RepositoryName: aws.String("dup-repo"),
					RepositoryArn: aws.String("arn:aws:ecr:us-east-1:123:repository/dup-repo")},
			},
		},
		listOutput: &ecr.ListImagesOutput{},
	}
	s := newTestScanner(emptyTaggingClient(), ecrClient, emptyS3Client(), []string{"dup-repo", "dup-repo"}, "")
	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	// Both entries point to the same name; only one resource.
	if len(inv.Resources) != 1 {
		t.Errorf("Scan() returned %d resources, want 1 (seen repo skipped)", len(inv.Resources))
	}
}

func TestScanEmptyARN(t *testing.T) {
	// A mapping with an empty ARN is skipped.
	tagging := &mockTaggingClient{
		outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
			{
				ResourceTagMappingList: []tagtypes.ResourceTagMapping{
					{ResourceARN: aws.String("")},
					{ResourceARN: aws.String("arn:aws:gamelift:us-east-1:123:fleet/fleet-abc")},
				},
			},
		},
	}
	s := newTestScanner(tagging, notFoundECRClient(), emptyS3Client(), nil, "")
	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Fatalf("Scan() returned %d resources, want 1", len(inv.Resources))
	}
}

func TestTitleCaseEmpty(t *testing.T) {
	if got := titleCase(""); got != "" {
		t.Errorf("titleCase(\"\") = %q, want empty", got)
	}
}

func TestNewScanner(t *testing.T) {
	awsCfg := aws.Config{Region: "us-east-1"}
	s := NewScanner(awsCfg, "us-west-2", []string{"a"}, "b")
	if s == nil {
		t.Fatal("NewScanner() returned nil")
	}
	if s.region != "us-west-2" {
		t.Errorf("region = %q, want us-west-2", s.region)
	}
	if len(s.ecrRepoNames) != 1 || s.ecrRepoNames[0] != "a" {
		t.Errorf("ecrRepoNames = %v, want [a]", s.ecrRepoNames)
	}
	if s.s3BucketPrefix != "b" {
		t.Errorf("s3BucketPrefix = %q, want b", s.s3BucketPrefix)
	}
}
