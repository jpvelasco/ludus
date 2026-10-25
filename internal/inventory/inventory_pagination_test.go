package inventory

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func imageIDs(digests ...string) []ecrtypes.ImageIdentifier {
	ids := make([]ecrtypes.ImageIdentifier, 0, len(digests))
	for _, d := range digests {
		ids = append(ids, ecrtypes.ImageIdentifier{ImageDigest: aws.String(d)})
	}
	return ids
}

func describedRepo() *ecr.DescribeRepositoriesOutput {
	return &ecr.DescribeRepositoriesOutput{
		Repositories: []ecrtypes.Repository{
			{
				RepositoryName: aws.String("ludus-server"),
				RepositoryArn:  aws.String("arn:aws:ecr:us-east-1:123:repository/ludus-server"),
			},
		},
	}
}

// TestScanECRImageDetailCountsAllPages pins #742: ListImages results are
// paginated with NextToken, and the reported image count must cover every
// page — not just the first.
func TestScanECRImageDetailCountsAllPages(t *testing.T) {
	ecrClient := &mockECRClient{
		describeOutput: describedRepo(),
		listOutputs: []*ecr.ListImagesOutput{
			{ImageIds: imageIDs("sha256:aaa", "sha256:bbb"), NextToken: aws.String("page-2")},
			{ImageIds: imageIDs("sha256:ccc", "sha256:ddd", "sha256:eee")},
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
		t.Errorf("detail = %q, want %q across both pages", inv.Resources[0].Detail, "5 images")
	}
	if len(ecrClient.listInputs) != 2 {
		t.Fatalf("ListImages calls = %d, want 2 (pagination must be followed)", len(ecrClient.listInputs))
	}
	if tok := aws.ToString(ecrClient.listInputs[1].NextToken); tok != "page-2" {
		t.Errorf("second ListImages NextToken = %q, want %q", tok, "page-2")
	}
}

// TestScanECRImageDetailPageErrorWarns pins the degraded-coverage path: a
// mid-pagination failure records a warning and reports no count rather than
// an undercount.
func TestScanECRImageDetailPageErrorWarns(t *testing.T) {
	ecrClient := &mockECRClient{
		describeOutput: describedRepo(),
		listOutputs: []*ecr.ListImagesOutput{
			{ImageIds: imageIDs("sha256:aaa"), NextToken: aws.String("page-2")},
		},
		listErr: &pageErrMarker{},
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
		t.Errorf("detail = %q, want empty on list failure", inv.Resources[0].Detail)
	}
	if len(inv.Warnings) == 0 || !strings.Contains(inv.Warnings[0], "ludus-server") {
		t.Errorf("warnings = %v, want a warning naming the repository", inv.Warnings)
	}
}

// pageErrMarker fails every ListImages call after the first page.
type pageErrMarker struct{}

func (*pageErrMarker) Error() string { return "page boom" }

func bucket(name string) s3types.Bucket {
	return s3types.Bucket{Name: aws.String(name)}
}

// TestScanS3BucketsCountsAllPages pins #742: ListBuckets paginates with a
// ContinuationToken; buckets on later pages must appear in the inventory.
func TestScanS3BucketsCountsAllPages(t *testing.T) {
	s3Client := &mockS3Client{
		listOutputs: []*s3.ListBucketsOutput{
			{Buckets: []s3types.Bucket{bucket("other-deployment")}, ContinuationToken: aws.String("page-2")},
			{Buckets: []s3types.Bucket{bucket("ludus-builds-mine")}},
		},
	}
	s := newTestScanner(emptyTaggingClient(), notFoundECRClient(), s3Client, nil, "ludus-builds-")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 1 {
		t.Fatalf("resources = %d, want 1 (bucket on the second page)", len(inv.Resources))
	}
	if inv.Resources[0].Name != "ludus-builds-mine" {
		t.Errorf("resource name = %q, want %q", inv.Resources[0].Name, "ludus-builds-mine")
	}
	if len(s3Client.listInputs) != 2 {
		t.Fatalf("ListBuckets calls = %d, want 2 (pagination must be followed)", len(s3Client.listInputs))
	}
	if tok := aws.ToString(s3Client.listInputs[1].ContinuationToken); tok != "page-2" {
		t.Errorf("second ListBuckets ContinuationToken = %q, want %q", tok, "page-2")
	}
}

// TestScanS3BucketsListErrorWarns keeps the existing degraded-coverage
// behavior: a list failure warns and skips rather than failing the scan.
func TestScanS3BucketsListErrorWarns(t *testing.T) {
	s3Client := &mockS3Client{listErr: &pageErrMarker{}}
	s := newTestScanner(emptyTaggingClient(), notFoundECRClient(), s3Client, nil, "ludus-builds-")

	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(inv.Resources) != 0 {
		t.Errorf("resources = %d, want 0", len(inv.Resources))
	}
	if len(inv.Warnings) == 0 || !strings.Contains(inv.Warnings[0], "list S3 buckets") {
		t.Errorf("warnings = %v, want a bucket-list warning", inv.Warnings)
	}
}
